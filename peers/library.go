package peers

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"

	"github.com/euforia/gossip/peers/peerspb"
)

// Peer holds the peer hard-state as well as soft-state
type Peer struct {
	*peerspb.Peer
	Distance time.Duration
}

// Library is a library of all known peers
type Library interface {
	// Get a peer by it's public key
	GetByPublicKey(...[]byte) []*peerspb.Peer
	// Get a peer by address
	GetByAddress(...string) []*peerspb.Peer
	// List all peers including local
	List() []*Peer
	// List in ascending order of distance
	Closest() []*Peer
	// Add a new peer
	Add(peers ...*peerspb.Peer) (int, error)
	// Update a peer
	Update(peers ...*peerspb.Peer) int
	// Set the local peer
	SetLocal(peer *peerspb.Peer)
	// Return the local peer
	Local() *peerspb.Peer
	// Mark the peer as being offline
	Offline(addr string) (*peerspb.Peer, bool)
	// Returns number of peers in the library including local
	Count() int
}

// InmemLibrary is an in-memory library to hold known peers
type InmemLibrary struct {
	// Lock peer list
	mu sync.RWMutex
	// All peers.  PublicKey is the primary key. First peer in the list is the
	// local peeer
	m []*Peer
}

// NewInmemLibrary inits a new in-memory peer library
func NewInmemLibrary() *InmemLibrary {
	return &InmemLibrary{
		m: make([]*Peer, 1),
	}
}

// SetLocal sets the local peer.  This has no lock as it is meant to be called
// once after the library as been initialized
func (pl *InmemLibrary) SetLocal(peer *peerspb.Peer) {
	pl.m[0] = &Peer{Peer: peer}
}

// Local returns the local peer
func (pl *InmemLibrary) Local() *peerspb.Peer {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	return pl.m[0].Peer
}

// Add adds the peers to the store by PublicKey.
func (pl *InmemLibrary) Add(peers ...*peerspb.Peer) (int, error) {
	var (
		n   int
		err error
	)

	pl.mu.Lock()

	for i, peer := range peers {
		pubkey := peer.PublicKey

		if len(pubkey) == 0 {
			err = errors.New("public key missing")
			continue
		}

		if _, _, ok := pl.getByPublicKey(pubkey); !ok {
			n++
			peers[i].LastSeen = time.Now().UnixNano()
			pl.m = append(pl.m, &Peer{Peer: peers[i]})
		}

	}

	pl.mu.Unlock()

	return n, err
}

// Count returns the total count of peers in the library
func (pl *InmemLibrary) Count() int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	return len(pl.m)
}

// Update updates the peer by its PublicKey
func (pl *InmemLibrary) Update(peers ...*peerspb.Peer) int {
	var c int

	pl.mu.Lock()

	// Update the peer
	for _, peer := range peers {

		i, _, ok := pl.getByPublicKey(peer.PublicKey)
		if !ok {
			continue
		}

		// Carry over the previous distance.  We will compute it later
		distance := pl.m[i].Distance
		pl.m[i] = &Peer{Peer: peer, Distance: distance}
		pl.m[i].LastSeen = time.Now().UnixNano()
		c++

	}

	pl.computeDistance()

	pl.mu.Unlock()

	return c
}

// computeDistance computes the distance to each peer
func (pl *InmemLibrary) computeDistance() {
	local := pl.m[0].Coordinate
	if local == nil {
		return
	}

	for _, peer := range pl.m[1:] {
		if peer.Coordinate != nil {
			peer.Distance = local.DistanceTo(peer.Coordinate)
		}
	}
}

// Offline sets the peer by the given address as offline
func (pl *InmemLibrary) Offline(addr string) (*peerspb.Peer, bool) {
	for i, p := range pl.m {
		if p.Address() == addr {
			pl.m[i].Offline = true
			return p.Clone(), true
		}
	}

	return nil, false
}

// List returns all peers in the library
func (pl *InmemLibrary) List() []*Peer {
	pl.mu.RLock()

	out := make([]*Peer, len(pl.m))
	copy(out, pl.m)

	pl.mu.RUnlock()

	return out
}

// GetByPublicKey returns a list of peers for the given public key.  If the
// public key is not found the slot in the list will be empty as such the return
// will always be of the same length as the input.  If no public keys are
// provided all nodes are returned.
func (pl *InmemLibrary) GetByPublicKey(pks ...[]byte) []*peerspb.Peer {
	out := make([]*peerspb.Peer, len(pks))

	pl.mu.RLock()

	for i, pk := range pks {
		if _, got, ok := pl.getByPublicKey(pk); ok {
			out[i] = got
		}
	}

	pl.mu.RUnlock()

	return out
}

// GetByAddress returns a list of peers by address.  The logic is the same as
// GetByPublicKey
func (pl *InmemLibrary) GetByAddress(addrs ...string) []*peerspb.Peer {
	out := make([]*peerspb.Peer, len(addrs))

	pl.mu.RLock()

	for i, addr := range addrs {
		if _, got, ok := pl.getByAddr(addr); ok {
			out[i] = got
		}
	}

	pl.mu.RUnlock()

	return out
}

// Snapshot returns a byte slice containing all peers in the library.  First
// item is the local peer
func (pl *InmemLibrary) Snapshot() ([]byte, error) {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	out := make([]byte, 0)
	for _, p := range pl.m {
		b, err := serialize(p)
		if err == nil {
			out = append(out, b...)
			continue
		}

		return nil, err
	}

	return out, nil
}

func serialize(p *Peer) ([]byte, error) {
	b, err := proto.Marshal(p)
	if err == nil {
		sz := make([]byte, 4)
		binary.BigEndian.PutUint32(sz, uint32(len(b)))
		return append(sz, b...), nil
	}

	return nil, err
}

// Restore populates the peer library with the snapshot byte stream
func (pl *InmemLibrary) Restore(b []byte) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	var (
		l = len(b)
		s int
		e = 4
	)

	list := make([]*Peer, 0)

	for e < l {
		var (
			size = binary.BigEndian.Uint32(b[s:e])
			data = b[e : e+int(size)]
			pb   peerspb.Peer
		)

		err := proto.Unmarshal(data, &pb)
		if err == nil {
			list = append(list, &Peer{Peer: &pb})
			s = e + int(size)
			e = s + 4
			continue
		}

		return err
	}

	pl.m = list

	return nil
}

func (pl *InmemLibrary) getByPublicKey(pk []byte) (int, *peerspb.Peer, bool) {

	for i, p := range pl.m {
		if bytes.Compare(p.PublicKey, pk) == 0 {
			return i, p.Clone(), true
		}
	}

	return -1, nil, false
}

func (pl *InmemLibrary) getByAddr(addr string) (int, *peerspb.Peer, bool) {

	for i, p := range pl.m {
		if p.Address() == addr {
			return i, p.Clone(), true
		}
	}

	return -1, nil, false
}
