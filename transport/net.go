package transport

import (
	"encoding/binary"
	"net"
	"time"
)

// MuxedListener is a muxed listener interface
type MuxedListener struct {
	addr net.Addr
	ch   <-chan net.Conn
}

// ListenTCP return a TCP MuxedListener
func ListenTCP(addr string, ch <-chan net.Conn) (*MuxedListener, error) {
	raddr, err := net.ResolveTCPAddr("tcp", addr)
	ln := &MuxedListener{raddr, ch}
	return ln, err
}

// Accept implements a muxed net.Listener
func (ln *MuxedListener) Accept() (net.Conn, error) {
	conn := <-ln.ch
	return conn, nil
}

// Close implements a muxed net.Listener
func (ln *MuxedListener) Close() error {
	return nil
}

// Addr implements a muxed net.Listener
func (ln *MuxedListener) Addr() net.Addr {
	return ln.addr
}

// TODO: Revisit how we do this
// Non-gossip memberlist connections.
type defaultNoMuxConn struct {
	readFirst bool
	first2    []byte
	net.Conn
}

func (conn *defaultNoMuxConn) Read(p []byte) (int, error) {
	// read pass-through if we've read the first byte
	if conn.readFirst {
		return conn.Conn.Read(p)
	}

	// Read marker bytes and internally mark we've read the first N bytes
	copy(p[:2], conn.first2)
	conn.readFirst = true

	// Read from connection
	n, err := conn.Conn.Read(p[2:])
	return n + 2, err
}

// DialTimeout is dialer for muxed connections.  It takes a remote address, mux id and
// connection timeout value
func DialTimeout(addr string, magic uint16, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}

	m := make([]byte, 2)
	binary.BigEndian.PutUint16(m, magic)

	_, err = conn.Write(m)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}
