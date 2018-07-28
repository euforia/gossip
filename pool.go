package gossip

import (
	"net"

	"github.com/euforia/gossip/peers"
	"github.com/euforia/gossip/peers/peerspb"
	"github.com/hashicorp/memberlist"
	"github.com/hexablock/log"
)

// Pool is a single gossip pool
type Pool struct {
	// pool id
	id int32

	// pool config
	mconf *memberlist.Config

	// memberlist for pool
	*memberlist.Memberlist

	// peers in the bool
	peers peers.Library

	// broadcasts
	delegate *delegate

	log *log.Logger
}

// NewPool returns a new gossip pool using the given config
func NewPool(conf *PoolConfig) *Pool {
	conf.Validate()

	local := &peerspb.Peer{
		Name: conf.Memberlist.Name,
		Addr: net.ParseIP(conf.Memberlist.AdvertiseAddr),
		Port: uint32(conf.Memberlist.AdvertisePort),
	}

	// COMBAK: Add a real public key
	local.PublicKey = []byte(local.Address())

	conf.Peers.SetLocal(local)

	delg := newDelegate(conf.ID, conf.Peers, conf.Delegate, conf.Logger)

	conf.Memberlist.Delegate = delg
	conf.Memberlist.Events = newEventsDelegate(conf.ID, conf.Peers, conf.Events, conf.Logger)
	conf.Memberlist.Ping = newPingDelegate(conf.Peers, conf.Vivaldi, conf.Logger)

	p := &Pool{
		id:       conf.ID,
		mconf:    conf.Memberlist,
		peers:    conf.Peers,
		delegate: delg,
		log:      conf.Logger,
	}

	return p
}

// Peers returns a peers library containing all peers in the pool
func (p *Pool) Peers() peers.Library {
	return p.peers
}

// Start creates the underlying memberlist object
func (p *Pool) Start() (err error) {
	p.Memberlist, err = memberlist.Create(p.mconf)
	if err == nil {
		p.log.Infof("Pool %d started!", p.id)
	}
	return
}

// Broadcast queues data to be broadcasted to the network via memberlist
func (p *Pool) Broadcast(data []byte) error {
	// Schedule the broadcast
	p.delegate.bcast <- data
	return nil
}
