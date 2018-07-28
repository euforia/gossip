package peerspb

import "fmt"

// Address returns the string representation of the peer address
func (peer *Peer) Address() string {
	return fmt.Sprintf("%s:%d", peer.Addr.String(), peer.Port)
}

func (peer *Peer) Clone() *Peer {

	p := *peer
	if p.Coordinate != nil {
		c := *peer.Coordinate
		p.Coordinate = &c
	}

	return &p
}
