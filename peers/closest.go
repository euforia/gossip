package peers

import "sort"

func (lib *InmemLibrary) Len() int {
	return len(lib.m)
}

func (lib *InmemLibrary) Less(i, j int) bool {
	return lib.m[i].Distance < lib.m[j].Distance
}

func (lib *InmemLibrary) Swap(i, j int) {
	lib.m[i], lib.m[j] = lib.m[j], lib.m[i]
}

// Closest returns all peers in ascending order of distance
func (lib *InmemLibrary) Closest() []*Peer {
	lib.mu.Lock()

	sort.Sort(lib)

	out := make([]*Peer, len(lib.m))
	copy(out, lib.m)

	lib.mu.Unlock()

	return out
}
