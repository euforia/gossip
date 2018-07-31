package peers

import "sort"

// Closest returns all peers in ascending order of distance
func (lib *InmemLibrary) Closest() []*Peer {
	lib.mu.Lock()

	sort.Sort(ClosestPeers(lib.m))

	out := make([]*Peer, len(lib.m))
	copy(out, lib.m)

	lib.mu.Unlock()

	return out
}

// ClosestPeers allows to sort a set of peers by distance
type ClosestPeers []*Peer

func (peers ClosestPeers) Len() int {
	return len(peers)
}

func (peers ClosestPeers) Less(i, j int) bool {
	return peers[i].Distance < peers[j].Distance
}

func (peers ClosestPeers) Swap(i, j int) {
	peers[i], peers[j] = peers[j], peers[i]
}
