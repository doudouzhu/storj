// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package monitor

import (
	"context"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"
	monkit "gopkg.in/spacemonkeygo/monkit.v2"

	"storj.io/storj/internal/sync2"
	"storj.io/storj/pkg/kademlia"
	"storj.io/storj/pkg/pb"
	"storj.io/storj/storagenode/bandwidth"
	"storj.io/storj/storagenode/pieces"
)

var (
	mon = monkit.Package()

	// Error is the default error class for piecestore monitor errors
	Error = errs.Class("piecestore monitor")
)

// Config defines parameters for storage node disk and bandwidth usage monitoring.
type Config struct {
	Interval time.Duration `help:"how frequently Kademlia bucket should be refreshed with node stats" default:"1h0m0s"`
}

// Service which monitors disk usage and updates kademlia network as necessary.
type Service struct {
	log                *zap.Logger
	routingTable       *kademlia.RoutingTable
	store              *pieces.Store
	pieceInfo          pieces.DB
	usageDB            bandwidth.DB
	allocatedDiskSpace int64
	allocatedBandwidth int64
	Loop               sync2.Cycle
}

// TODO: should it be responsible for monitoring actual bandwidth as well?

// NewService creates a new storage node monitoring service.
func NewService(log *zap.Logger, routingTable *kademlia.RoutingTable, store *pieces.Store, pieceInfo pieces.DB, usageDB bandwidth.DB, allocatedDiskSpace, allocatedBandwidth int64, interval time.Duration) *Service {
	return &Service{
		log:                log,
		routingTable:       routingTable,
		store:              store,
		pieceInfo:          pieceInfo,
		usageDB:            usageDB,
		allocatedDiskSpace: allocatedDiskSpace,
		allocatedBandwidth: allocatedBandwidth,
		Loop:               *sync2.NewCycle(interval),
	}
}

// Run runs monitor service
func (service *Service) Run(ctx context.Context) (err error) {
	defer mon.Task()(&ctx)(&err)

	// get the disk space details
	// The returned path ends in a slash only if it represents a root directory, such as "/" on Unix or `C:\` on Windows.
	storageStatus, err := service.store.StorageStatus()
	if err != nil {
		return Error.Wrap(err)
	}
	freeDiskSpace := storageStatus.DiskFree

	totalUsed, err := service.usedSpace(ctx)
	if err != nil {
		return err
	}

	usedBandwidth, err := service.usedBandwidth(ctx)
	if err != nil {
		return err
	}

	if usedBandwidth > service.allocatedBandwidth {
		service.log.Warn("Exceed the allowed Bandwidth setting")
	} else {
		service.log.Info("Remaining Bandwidth", zap.Int64("bytes", service.allocatedBandwidth-usedBandwidth))
	}

	// check your hard drive is big enough
	// first time setup as a piece node server
	if totalUsed == 0 && freeDiskSpace < service.allocatedDiskSpace {
		service.allocatedDiskSpace = freeDiskSpace
		service.log.Warn("Disk space is less than requested. Allocating space", zap.Int64("bytes", service.allocatedDiskSpace))
	}

	// on restarting the Piece node server, assuming already been working as a node
	// used above the alloacated space, user changed the allocation space setting
	// before restarting
	if totalUsed >= service.allocatedDiskSpace {
		service.log.Warn("Used more space than allocated. Allocating space", zap.Int64("bytes", service.allocatedDiskSpace))
	}

	// the available disk space is less than remaining allocated space,
	// due to change of setting before restarting
	if freeDiskSpace < service.allocatedDiskSpace-totalUsed {
		service.allocatedDiskSpace = freeDiskSpace + totalUsed
		service.log.Warn("Disk space is less than requested. Allocating space", zap.Int64("bytes", service.allocatedDiskSpace))
	}

	return service.Loop.Run(ctx, func(ctx context.Context) error {
		err := service.updateNodeInformation(ctx)
		if err != nil {
			service.log.Error("error during updating node information: ", zap.Error(err))
		}
		return err
	})
}

// Close stops the monitor service.
func (service *Service) Close() (err error) {
	service.Loop.Close()
	return nil
}

func (service *Service) updateNodeInformation(ctx context.Context) error {
	usedSpace, err := service.usedSpace(ctx)
	if err != nil {
		return Error.Wrap(err)
	}

	usedBandwidth, err := service.usedBandwidth(ctx)
	if err != nil {
		return Error.Wrap(err)
	}

	service.routingTable.UpdateSelfCapacity(&pb.NodeCapacity{
		FreeBandwidth: service.allocatedBandwidth - usedBandwidth,
		FreeDisk:      service.allocatedDiskSpace - usedSpace,
	})

	return nil
}

func (service *Service) usedSpace(ctx context.Context) (int64, error) {
	usedSpace, err := service.pieceInfo.SpaceUsed(ctx)
	if err != nil {
		return 0, err
	}
	return usedSpace, nil
}

func (service *Service) usedBandwidth(ctx context.Context) (int64, error) {
	usage, err := bandwidth.TotalMonthlySummary(ctx, service.usageDB)
	if err != nil {
		return 0, err
	}
	return usage.Total(), nil
}

// AvailableSpace returns available disk space for upload
func (service *Service) AvailableSpace(ctx context.Context) (int64, error) {
	usedSpace, err := service.pieceInfo.SpaceUsed(ctx)
	if err != nil {
		return 0, Error.Wrap(err)
	}
	allocatedSpace := service.allocatedDiskSpace
	return allocatedSpace - usedSpace, nil
}

// AvailableBandwidth returns available bandwidth for upload/download
func (service *Service) AvailableBandwidth(ctx context.Context) (int64, error) {
	usage, err := bandwidth.TotalMonthlySummary(ctx, service.usageDB)
	if err != nil {
		return 0, Error.Wrap(err)
	}
	allocatedBandwidth := service.allocatedBandwidth
	return allocatedBandwidth - usage.Total(), nil
}
