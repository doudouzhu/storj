// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package overlay

import (
	"context"
	"errors"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/storj/pkg/pb"
	"storj.io/storj/pkg/storj"
	"storj.io/storj/storage"
)

// ErrEmptyNode is returned when the nodeID is empty
var ErrEmptyNode = errs.New("empty node ID")

// ErrNodeNotFound is returned if a node does not exist in database
var ErrNodeNotFound = errs.Class("node not found")

// ErrBucketNotFound is returned if a bucket is unable to be found in the routing table
var ErrBucketNotFound = errs.New("bucket not found")

// ErrNotEnoughNodes is when selecting nodes failed with the given parameters
var ErrNotEnoughNodes = errs.Class("not enough nodes")

// OverlayError creates class of errors for stack traces
var OverlayError = errs.Class("overlay error")

// DB implements the database for overlay.Cache
type DB interface {
	// SelectStorageNodes looks up nodes based on criteria
	SelectStorageNodes(ctx context.Context, count int, criteria *NodeCriteria) ([]*pb.Node, error)
	// SelectNewStorageNodes looks up nodes based on new node criteria
	SelectNewStorageNodes(ctx context.Context, count int, criteria *NodeCriteria) ([]*pb.Node, error)

	// Get looks up the node by nodeID
	Get(ctx context.Context, nodeID storj.NodeID) (*NodeDossier, error)
	// KnownUnreliableOrOffline filters a set of nodes to unhealth or offlines node, independent of new
	KnownUnreliableOrOffline(context.Context, *NodeCriteria, storj.NodeIDList) (storj.NodeIDList, error)
	// Paginate will page through the database nodes
	Paginate(ctx context.Context, offset int64, limit int) ([]*NodeDossier, bool, error)

	// CreateStats initializes the stats for node.
	CreateStats(ctx context.Context, nodeID storj.NodeID, initial *NodeStats) (stats *NodeStats, err error)
	// Update updates node address
	UpdateAddress(ctx context.Context, value *pb.Node) error
	// UpdateStats all parts of single storagenode's stats.
	UpdateStats(ctx context.Context, request *UpdateRequest) (stats *NodeStats, err error)
	// UpdateNodeInfo updates node dossier with info requested from the node itself like node type, email, wallet, capacity, and version.
	UpdateNodeInfo(ctx context.Context, node storj.NodeID, nodeInfo *pb.InfoResponse) (stats *NodeDossier, err error)
	// UpdateUptime updates a single storagenode's uptime stats.
	UpdateUptime(ctx context.Context, nodeID storj.NodeID, isUp bool) (stats *NodeStats, err error)
}

// FindStorageNodesRequest defines easy request parameters.
type FindStorageNodesRequest struct {
	MinimumRequiredNodes int
	RequestedCount       int
	FreeBandwidth        int64
	FreeDisk             int64
	ExcludedNodes        []storj.NodeID
	MinimumVersion       string // semver or empty
}

// NodeCriteria are the requirements for selecting nodes
type NodeCriteria struct {
	FreeBandwidth      int64
	FreeDisk           int64
	AuditCount         int64
	AuditSuccessRatio  float64
	UptimeCount        int64
	UptimeSuccessRatio float64
	Excluded           []storj.NodeID
	MinimumVersion     string // semver or empty
	OnlineWindow       time.Duration
}

// UpdateRequest is used to update a node status.
type UpdateRequest struct {
	NodeID       storj.NodeID
	AuditSuccess bool
	IsUp         bool
}

// NodeDossier is the complete info that the satellite tracks for a storage node
type NodeDossier struct {
	pb.Node
	Type       pb.NodeType
	Operator   pb.NodeOperator
	Capacity   pb.NodeCapacity
	Reputation NodeStats
	Version    pb.NodeVersion
}

// NodeStats contains statistics about a node.
type NodeStats struct {
	Latency90          int64
	AuditSuccessRatio  float64
	AuditSuccessCount  int64
	AuditCount         int64
	UptimeRatio        float64
	UptimeSuccessCount int64
	UptimeCount        int64
	LastContactSuccess time.Time
	LastContactFailure time.Time
}

// Cache is used to store and handle node information
type Cache struct {
	log         *zap.Logger
	db          DB
	preferences NodeSelectionConfig
}

// NewCache returns a new Cache
func NewCache(log *zap.Logger, db DB, preferences NodeSelectionConfig) *Cache {
	return &Cache{
		log:         log,
		db:          db,
		preferences: preferences,
	}
}

// Close closes resources
func (cache *Cache) Close() error { return nil }

// Inspect lists limited number of items in the cache
func (cache *Cache) Inspect(ctx context.Context) (storage.Keys, error) {
	// TODO: implement inspection tools
	return nil, errors.New("not implemented")
}

// Paginate returns a list of `limit` nodes starting from `start` offset.
func (cache *Cache) Paginate(ctx context.Context, offset int64, limit int) (_ []*NodeDossier, _ bool, err error) {
	defer mon.Task()(&ctx)(&err)
	return cache.db.Paginate(ctx, offset, limit)
}

// Get looks up the provided nodeID from the overlay cache
func (cache *Cache) Get(ctx context.Context, nodeID storj.NodeID) (_ *NodeDossier, err error) {
	defer mon.Task()(&ctx)(&err)
	if nodeID.IsZero() {
		return nil, ErrEmptyNode
	}
	return cache.db.Get(ctx, nodeID)
}

// IsOnline checks if a node is 'online' based on the collected statistics.
func (cache *Cache) IsOnline(node *NodeDossier) bool {
	return time.Now().Sub(node.Reputation.LastContactSuccess) < cache.preferences.OnlineWindow &&
		node.Reputation.LastContactSuccess.After(node.Reputation.LastContactFailure)
}

// FindStorageNodes searches the overlay network for nodes that meet the provided requirements
func (cache *Cache) FindStorageNodes(ctx context.Context, req FindStorageNodesRequest) ([]*pb.Node, error) {
	return cache.FindStorageNodesWithPreferences(ctx, req, &cache.preferences)
}

// FindStorageNodesWithPreferences searches the overlay network for nodes that meet the provided criteria
func (cache *Cache) FindStorageNodesWithPreferences(ctx context.Context, req FindStorageNodesRequest, preferences *NodeSelectionConfig) (nodes []*pb.Node, err error) {
	defer mon.Task()(&ctx)(&err)

	// TODO: add sanity limits to requested node count
	// TODO: add sanity limits to excluded nodes
	reputableNodeCount := req.MinimumRequiredNodes
	if reputableNodeCount <= 0 {
		reputableNodeCount = req.RequestedCount
	}

	excluded := req.ExcludedNodes

	newNodeCount := 0
	if preferences.NewNodePercentage > 0 {
		newNodeCount = int(float64(reputableNodeCount) * preferences.NewNodePercentage)
	}

	var newNodes []*pb.Node
	if newNodeCount > 0 {
		newNodes, err = cache.db.SelectNewStorageNodes(ctx, newNodeCount, &NodeCriteria{
			FreeBandwidth:     req.FreeBandwidth,
			FreeDisk:          req.FreeDisk,
			AuditCount:        preferences.AuditCount,
			AuditSuccessRatio: preferences.AuditSuccessRatio,
			Excluded:          excluded,
			MinimumVersion:    preferences.MinimumVersion,
			OnlineWindow:      preferences.OnlineWindow,
		})
		if err != nil {
			return nil, err
		}
	}

	// add selected new nodes to the excluded list for reputable node selection
	for _, newNode := range newNodes {
		excluded = append(excluded, newNode.Id)
	}

	criteria := NodeCriteria{
		FreeBandwidth:      req.FreeBandwidth,
		FreeDisk:           req.FreeDisk,
		AuditCount:         preferences.AuditCount,
		AuditSuccessRatio:  preferences.AuditSuccessRatio,
		UptimeCount:        preferences.UptimeCount,
		UptimeSuccessRatio: preferences.UptimeRatio,
		Excluded:           excluded,
		MinimumVersion:     preferences.MinimumVersion,
		OnlineWindow:       preferences.OnlineWindow,
	}
	reputableNodes, err := cache.db.SelectStorageNodes(ctx, reputableNodeCount-len(newNodes), &criteria)
	if err != nil {
		return nil, err
	}

	nodes = append(nodes, newNodes...)
	nodes = append(nodes, reputableNodes...)

	if len(nodes) < reputableNodeCount {
		return nodes, ErrNotEnoughNodes.New("requested %d found %d; %+v ", reputableNodeCount, len(nodes), criteria)
	}

	return nodes, nil
}

// KnownUnreliableOrOffline filters a set of nodes to unhealth or offlines node, independent of new.
func (cache *Cache) KnownUnreliableOrOffline(ctx context.Context, nodeIds storj.NodeIDList) (badNodes storj.NodeIDList, err error) {
	defer mon.Task()(&ctx)(&err)
	criteria := &NodeCriteria{
		AuditCount:         cache.preferences.AuditCount,
		AuditSuccessRatio:  cache.preferences.AuditSuccessRatio,
		OnlineWindow:       cache.preferences.OnlineWindow,
		UptimeCount:        cache.preferences.UptimeCount,
		UptimeSuccessRatio: cache.preferences.UptimeRatio,
	}
	return cache.db.KnownUnreliableOrOffline(ctx, criteria, nodeIds)
}

// Put adds a node id and proto definition into the overlay cache
func (cache *Cache) Put(ctx context.Context, nodeID storj.NodeID, value pb.Node) (err error) {
	defer mon.Task()(&ctx)(&err)

	// If we get a Node without an ID (i.e. bootstrap node)
	// we don't want to add to the routing tbale
	if nodeID.IsZero() {
		return nil
	}
	if nodeID != value.Id {
		return errors.New("invalid request")
	}
	return cache.db.UpdateAddress(ctx, &value)
}

// Create adds a new stats entry for node.
func (cache *Cache) Create(ctx context.Context, nodeID storj.NodeID, initial *NodeStats) (stats *NodeStats, err error) {
	defer mon.Task()(&ctx)(&err)
	return cache.db.CreateStats(ctx, nodeID, initial)
}

// UpdateStats all parts of single storagenode's stats.
func (cache *Cache) UpdateStats(ctx context.Context, request *UpdateRequest) (stats *NodeStats, err error) {
	defer mon.Task()(&ctx)(&err)
	return cache.db.UpdateStats(ctx, request)
}

// UpdateNodeInfo updates node dossier with info requested from the node itself like node type, email, wallet, capacity, and version.
func (cache *Cache) UpdateNodeInfo(ctx context.Context, node storj.NodeID, nodeInfo *pb.InfoResponse) (stats *NodeDossier, err error) {
	defer mon.Task()(&ctx)(&err)
	return cache.db.UpdateNodeInfo(ctx, node, nodeInfo)
}

// UpdateUptime updates a single storagenode's uptime stats.
func (cache *Cache) UpdateUptime(ctx context.Context, nodeID storj.NodeID, isUp bool) (stats *NodeStats, err error) {
	defer mon.Task()(&ctx)(&err)
	return cache.db.UpdateUptime(ctx, nodeID, isUp)
}

// ConnFailure implements the Transport Observer `ConnFailure` function
func (cache *Cache) ConnFailure(ctx context.Context, node *pb.Node, failureError error) {
	var err error
	defer mon.Task()(&ctx)(&err)

	// TODO: Kademlia paper specifies 5 unsuccessful PINGs before removing the node
	// from our routing table, but this is the cache so maybe we want to treat
	// it differently.
	_, err = cache.db.UpdateUptime(ctx, node.Id, false)
	if err != nil {
		zap.L().Debug("error updating uptime for node", zap.Error(err))
	}
}

// ConnSuccess implements the Transport Observer `ConnSuccess` function
func (cache *Cache) ConnSuccess(ctx context.Context, node *pb.Node) {
	var err error
	defer mon.Task()(&ctx)(&err)

	err = cache.Put(ctx, node.Id, *node)
	if err != nil {
		zap.L().Debug("error updating uptime for node", zap.Error(err))
	}
	_, err = cache.db.UpdateUptime(ctx, node.Id, true)
	if err != nil {
		zap.L().Debug("error updating node connection info", zap.Error(err))
	}
}

// GetMissingPieces returns the list of offline nodes
func (cache *Cache) GetMissingPieces(ctx context.Context, pieces []*pb.RemotePiece) (missingPieces []int32, err error) {
	var nodeIDs storj.NodeIDList
	for _, p := range pieces {
		nodeIDs = append(nodeIDs, p.NodeId)
	}
	badNodeIDs, err := cache.KnownUnreliableOrOffline(ctx, nodeIDs)
	if err != nil {
		return nil, Error.New("error getting nodes %s", err)
	}

	for _, p := range pieces {
		for _, nodeID := range badNodeIDs {
			if nodeID == p.NodeId {
				missingPieces = append(missingPieces, p.GetPieceNum())
			}
		}
	}
	return missingPieces, nil
}
