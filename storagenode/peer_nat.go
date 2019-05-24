package storagenode

import (
	"context"
	"go.uber.org/zap"
	//"storj.io/storj/pkg/nat"
	"storj.io/storj/pkg/overlay"
	//"storj.io/storj/internal/sync2"
	"net"
	"time"
)

func (peer *Peer) NatRefresh() error {
	var err error
	var externalAddr net.Addr
	var curPaddress string
	var node overlay.NodeDossier

	node = peer.Kademlia.Service.Local()
	if externalAddr, err = peer.Mapping.ExternalAddr(); err != nil {
		peer.Log.Warn("get mapping externalAddr fail", zap.Error(err))
	} else if curPaddress = externalAddr.String(); false {
	} else if node.Paddress.Address == curPaddress {
	} else {
		peer.Log.Warn("UpdateSelfPaddress",
			zap.String("old paddress", node.Paddress.Address),
			zap.String("cur paddress", curPaddress))
		peer.Kademlia.RoutingTable.UpdateSelfPaddress(curPaddress)
	}

	return err
}

func (peer *Peer) RunNatRefresh(ctx context.Context) error {
	peer.RefreshNat.SetInterval(5 * time.Minute)

	return peer.RefreshNat.Run(ctx, func(ctx context.Context) error {
		return peer.NatRefresh()
	})
}
