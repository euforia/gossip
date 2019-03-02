package transport

import (
	"net"
	"time"

	"github.com/hashicorp/memberlist"
)

// poolTransport is the transport for a single admin pool.  It sits on top of
// the NetTransport to allow running multiple gossip pools on a single transport
type poolTransport struct {
	// Gossip pool id
	poolID uint8
	net    *NetTransport

	memlistCh chan net.Conn
	packetCh  chan *memberlist.Packet
}

func newPoolTransport(id uint8, trans *NetTransport) *poolTransport {
	return &poolTransport{
		poolID:    id,
		net:       trans,
		memlistCh: make(chan net.Conn),
		packetCh:  make(chan *memberlist.Packet),
	}
}

// FinalAdvertiseAddr satisfies memberlist.Transport
func (t *poolTransport) FinalAdvertiseAddr(ip string, port int) (net.IP, int, error) {
	return t.net.FinalAdvertiseAddr(ip, port)
}

// PacketCh implements memberlist.Transport. See Transport.
func (t *poolTransport) PacketCh() <-chan *memberlist.Packet {
	return t.packetCh
}

// StreamCh satisfies memberlist.Transport
func (t *poolTransport) StreamCh() <-chan net.Conn {
	return t.memlistCh
}

// WriteTo Satisfies memberlist.Transport. UDP write. See Transport.
func (t *poolTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return time.Time{}, err
	}

	// We made sure there's at least one UDP listener, so just use the
	// packet sending interface on the first one. Take the time after the
	// write call comes back, which will underestimate the time a little,
	// but help account for any delays before the write occurs.
	data := append([]byte{memlistMagic, t.poolID}, b...)
	_, err = t.net.udpListeners[0].WriteTo(data, udpAddr)
	return time.Now(), err
}

// DialTimeout satisfies memberlist.Transport used for memberlist mux.
func (t *poolTransport) DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Write memlist magic
	if _, err = conn.Write([]byte{memlistMagic, t.poolID}); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Shutdown satisfies memberlist.Transport. See Transport.
func (t *poolTransport) Shutdown() error {
	return nil
}

// this is called by the parent transport
func (t *poolTransport) close() {
	close(t.memlistCh)
	close(t.packetCh)
}
