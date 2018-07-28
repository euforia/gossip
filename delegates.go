package gossip

import (
	"encoding/binary"
	"net"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/memberlist"

	"github.com/hexablock/log"
	"github.com/hexablock/vivaldi"

	"github.com/euforia/gossip/peers"
	"github.com/euforia/gossip/peers/peerspb"
)

// Delegate is the overwritten memberlist delegate
type Delegate interface {
	NotifyMsg(msg []byte)
	MergeRemoteState(buf []byte, join bool)
	LocalState(join bool) []byte
}

// EventsDelegate implements in interface called on member events
type EventsDelegate interface {
	NotifyJoin(*peerspb.Peer)
	NotifyUpdate(*peerspb.Peer)
	NotifyLeave(*peerspb.Peer)
}

type noopDelegate struct{}

func (del *noopDelegate) NotifyMsg(msg []byte)                   {}
func (del *noopDelegate) MergeRemoteState(buf []byte, join bool) {}
func (del *noopDelegate) LocalState(join bool) []byte            { return nil }

type noopEventsDelegate struct{}

func (del *noopEventsDelegate) NotifyJoin(*peerspb.Peer)   {}
func (del *noopEventsDelegate) NotifyUpdate(*peerspb.Peer) {}
func (del *noopEventsDelegate) NotifyLeave(*peerspb.Peer)  {}

type delegate struct {
	// gossip pool id
	poolID int32

	// used for state
	addr net.IP
	port [4]byte

	// peers library for the pool
	peers peers.Library
	// Broadcast channel
	bcast chan []byte
	// User defined
	child Delegate
	// Logger
	log *log.Logger
}

func newDelegate(id int32, peersLib peers.Library, del Delegate, logger *log.Logger) *delegate {
	d := &delegate{
		poolID: id,
		peers:  peersLib,
		bcast:  make(chan []byte, 16),
		child:  del,
		log:    logger,
	}
	if d.child == nil {
		d.child = &noopDelegate{}
	}

	local := d.peers.Local()
	d.addr = local.Addr
	binary.BigEndian.PutUint32(d.port[:], local.Port)

	return d
}

func (g *delegate) NodeMeta(limit int) []byte {
	local := g.peers.Local()
	b, err := proto.Marshal(local)
	if err == nil {
		return b
	}

	g.log.Errorf("Failed build node meta: '%v' pool=%d", err, g.poolID)
	return nil
}

// NotifyMsg is a direct pass-through to the child delegate
func (g *delegate) NotifyMsg(msg []byte) {
	g.child.NotifyMsg(msg)
}

// GetBroadcasts is used by memberlist to broadcast available data
func (g *delegate) GetBroadcasts(overhead, limit int) [][]byte {
	// Get channel length
	l := len(g.bcast)
	if l == 0 {
		return nil
	}

	// Read all data from the channel.
	out := make([][]byte, 0, l)
	for i := 0; i < l; i++ {
		b := <-g.bcast
		out = append(out, b)
	}

	return out
}

func (g *delegate) MergeRemoteState(buf []byte, join bool) {
	// 24 byte header
	ip, port, poolID := g.parseStateHeader(buf[:24])
	g.log.Debugf("State received pool=%d peer=%s:%d", poolID, ip.String(), port)

	g.child.MergeRemoteState(buf[24:], join)
}

// LocalState returns the local state. This is then sent to other peers made available
// in the MergeRemoteState call
func (g *delegate) LocalState(join bool) []byte {
	header := g.getStateHeader()
	user := g.child.LocalState(join)
	return append(header, user...)
}

func (g *delegate) parseStateHeader(header []byte) (net.IP, uint32, int32) {

	ip := net.IP(header[:16])
	port := binary.BigEndian.Uint32(header[16:20])
	poolID := binary.BigEndian.Uint32(header[20:24])

	return ip, port, int32(poolID)
}

func (g *delegate) getStateHeader() []byte {
	header := append(g.addr, g.port[:]...)
	poolID := make([]byte, 4)
	binary.BigEndian.PutUint32(poolID, uint32(g.poolID))
	return append(header, poolID...)
}

type eventsDelegate struct {
	poolID int32

	peers peers.Library

	child EventsDelegate

	log *log.Logger
}

func newEventsDelegate(id int32, peers peers.Library, del EventsDelegate, logger *log.Logger) memberlist.EventDelegate {
	ed := &eventsDelegate{id, peers, del, logger}
	if ed.child == nil {
		ed.child = &noopEventsDelegate{}
	}
	return ed
}

// NotifyJoin adds the newly joined node to the kelips dht
func (ged *eventsDelegate) NotifyJoin(node *memberlist.Node) {
	var peer peerspb.Peer

	err := proto.Unmarshal(node.Meta, &peer)
	if err != nil {
		ged.log.Errorf("NotifyJoin failed to unmarshal meta pool=%d error='%v'",
			ged.poolID, err)
		return
	}

	ged.log.Infof("Peer joined pool=%d name=%s address=%s count=%d error='%v'",
		ged.poolID, node.Name, peer.Address(), ged.peers.Count(), err)

	_, err = ged.peers.Add(&peer)
	if err == nil {
		ged.child.NotifyJoin(&peer)
	}

	return

}

// NotifyUpdate satisfies the memberlist event delegate
func (ged *eventsDelegate) NotifyUpdate(node *memberlist.Node) {
	var peer peerspb.Peer

	err := proto.Unmarshal(node.Meta, &peer)
	if err != nil {
		ged.log.Error(err)
		return
	}

	ged.peers.Update(&peer)

	ged.log.Infof("Peer updated name=%s address=%s", node.Name, peer.Address())

	ged.child.NotifyUpdate(&peer)
}

// NotifyLeave satisfies the memberlist event delegate. It marks the node as
// offline
func (ged *eventsDelegate) NotifyLeave(node *memberlist.Node) {
	var peer peerspb.Peer

	err := proto.Unmarshal(node.Meta, &peer)
	if err != nil {
		ged.log.Error(err)
		return
	}

	if np, ok := ged.peers.Offline(peer.Address()); ok {
		ged.log.Infof("Peer offlined name=%s address=%s", node.Name, peer.Address())
		ged.child.NotifyLeave(np)
	}
}

type pingDelegate struct {
	host  string
	coord *vivaldi.Client
	peers peers.Library
	log   *log.Logger
}

func newPingDelegate(peers peers.Library, vclient *vivaldi.Client, logger *log.Logger) *pingDelegate {
	lpeer := peers.Local()
	return &pingDelegate{
		host:  lpeer.Address(),
		peers: peers,
		log:   logger,
		coord: vclient,
	}
}

// AckPayload sends local node coordinates. It satisfies the PingDelegate
// interface
func (del *pingDelegate) AckPayload() []byte {
	coord := del.coord.GetCoordinate()
	b, err := proto.Marshal(coord)
	if err != nil {
		del.log.Errorf("Failed marshal coordinate: %v", err)
	}
	return b
}

// NotifyPingComplete updates local coordinate based on remote information. It
// satisfies the PingDelegate interface
func (del *pingDelegate) NotifyPingComplete(node *memberlist.Node, rtt time.Duration, payload []byte) {

	var other vivaldi.Coordinate
	err := proto.Unmarshal(payload, &other)
	if err != nil {
		del.log.Errorf("Failed marshal coordinate: %v", err)
		return
	}

	if rtt <= 0 {
		return
	}

	if err = del.updateCoords(node.Address(), other, rtt); err != nil {
		del.log.Errorf("Failed to update coordinate: %v", err)
	}

}

func (del *pingDelegate) updateCoords(addr string, other vivaldi.Coordinate, rtt time.Duration) error {
	remoteCoord := other.Clone()
	newLocalCoord, err := del.coord.Update(addr, remoteCoord, rtt)
	if err != nil {
		return err
	}

	// Get local and received peer to update coords
	peers := del.peers.GetByAddress(del.host, addr)

	// Local
	peers[0].Coordinate = newLocalCoord
	// Remote
	peers[1].Coordinate = remoteCoord

	// Update library
	del.peers.Update(peers...)

	return nil
}
