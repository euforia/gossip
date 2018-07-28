package gossip

import (
	"net"

	"github.com/hexablock/log"
	"github.com/hexablock/vivaldi"

	"github.com/euforia/gossip/transport"
	"github.com/hashicorp/memberlist"
)

// Gossip is the top-level gossip struct to manage multiple pools
type Gossip struct {
	// Unique node name
	name string

	// Adv address for state exchange
	advAddr string
	// Adv port for state exchange
	advPort int

	// Vivaldi coordinate client
	coord *vivaldi.Client

	// All gossip pools
	pools map[int32]*Pool

	// Core network transport
	trans *transport.NetTransport

	// Shared logger
	log *log.Logger

	// Debug to turn on underlying loggers
	debug bool
}

// New returns a new Gossip instance based on the given config
func New(conf *Config) (*Gossip, error) {
	conf.Validate()

	g := &Gossip{
		name:    conf.Name,
		advAddr: conf.AdvertiseAddr,
		advPort: conf.AdvertisePort,
		pools:   make(map[int32]*Pool, 1),
		log:     conf.Logger,
		debug:   conf.Debug,
	}

	// TODO: Change to use bind address
	transConf := &transport.Config{
		BindAddrs: []string{conf.BindAddr},
		BindPort:  conf.BindPort,
	}

	var err error

	g.trans, err = transport.NewNetTransport(transConf)
	if err != nil {
		return nil, err
	}

	g.coord, err = vivaldi.NewClient(conf.Coordinate)
	if err != nil {
		return nil, err
	}

	return g, nil
}

// GetPool returns a gossip pool by the given id.  It returns nil if the
// pool id does not exist
func (g *Gossip) GetPool(id int32) *Pool {
	p, _ := g.pools[id]
	return p
}

// ListPools returns a slice of int32 pool ids
func (g *Gossip) ListPools() []int32 {
	out := make([]int32, 0, len(g.pools))
	for k := range g.pools {
		out = append(out, k)
	}
	return out
}

// RegisterPool registers ie. creates a new pool with the given config.  All pools must be
// registered before gossip is started as the addition of pools is not thread safe
func (g *Gossip) RegisterPool(pconf *PoolConfig) *Pool {

	pconf.Vivaldi = g.coord
	pconf.Memberlist.Transport = g.trans.RegisterPool(uint8(pconf.ID))
	pconf.Memberlist.Name = g.name
	pconf.Memberlist.BindAddr = g.advAddr
	pconf.Memberlist.BindPort = g.advPort
	pconf.Memberlist.AdvertiseAddr = g.advAddr
	pconf.Memberlist.AdvertisePort = g.advPort
	pconf.Logger = g.log
	pconf.Debug = g.debug

	p := NewPool(pconf)
	g.pools[pconf.ID] = p
	return p
}

// Start starts the underlying transport and all registered gossip pools
// This should be called only after all pools have been registered
func (g *Gossip) Start() (err error) {
	g.trans.Start()

	for _, p := range g.pools {
		if err = p.Start(); err != nil {
			break
		}
	}

	return
}

// Listener returns a TCP net.Listener interface that can be used to allow the user
// to run other protocols
func (g *Gossip) Listener() net.Listener {
	ch := g.trans.TCPCh()
	ln, _ := transport.ListenTCP(g.advAddr, ch)
	return ln
}

// TCPConnections returns a read-only channel of incoming non-gossip tcp
// connections.  This is useful for custom application transport allowing
// network communication on a single port
func (g *Gossip) TCPConnections() <-chan net.Conn {
	return g.trans.TCPCh()
}

// UDPPackets returns a read-only channel incoming non-gossip udp packets. This
// is useful for custom application transport allowing network communication
// on a single port
func (g *Gossip) UDPPackets() <-chan *memberlist.Packet {
	return g.trans.UDPCh()
}
