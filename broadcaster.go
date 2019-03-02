package gossip

// broadcaster implements a memberlist.Delegate to provide a broadcast
// interface
type broadcaster struct {
	broadcast chan []byte
	Delegate
}

// GetBroadcasts is called when user data messages can be broadcast.
// It can return a list of buffers to send. Each buffer should assume an
// overhead as provided with a limit on the total byte size allowed.
// The total byte size of the resulting data to send must not exceed
// the limit. Care should be taken that this method does not block,
// since doing so would block the entire UDP packet receive loop.
//
// This provides a wrapper to the native method to provide non-blocking
// via channels.  It drains the complete channel and sends all of the
// data
func (bc *broadcaster) GetBroadcasts(overhead, limit int) [][]byte {
	buffSize := len(bc.broadcast)
	out := make([][]byte, buffSize)
	for i := 0; i < buffSize; i++ {
		out[i] = <-bc.broadcast
	}
	return out
}
