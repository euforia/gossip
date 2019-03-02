package transport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armon/go-metrics"
	sockaddr "github.com/hashicorp/go-sockaddr"
	"github.com/hashicorp/memberlist"
)

const (
	// udpPacketBufSize is used to buffer incoming packets during read
	// operations. An extra 2 byte is for the magic number
	udpPacketBufSize = 65534
	//udpPacketBufSize = 65536

	// udpRecvBufSize is a large buffer size that we attempt to set UDP
	// sockets to in order to handle a large volume of messages.
	udpRecvBufSize = 2 * 1024 * 1024
)

const (
	memlistMagic = 0xfe // Magic number for memberlist TCP connection
)

// Config is used to configure a net transport.
type Config struct {
	// BindAddrs is a list of addresses to bind to for both TCP and UDP
	// communications.
	BindAddrs []string

	// BindPort is the port to listen on, for each address above.
	BindPort int

	// Logger is a logger for operator messages.
	Logger *log.Logger
}

// NetTransport is a Transport implementation that uses connectionless UDP for
// packet operations, and ad-hoc TCP connections for stream operations. It also
// exposes channels to allow TCP and UDP connections for user defined transports
type NetTransport struct {
	config *Config

	// Transport per gossip pools
	pools map[uint8]*poolTransport

	// Additional muxed conns
	muxed map[uint16]chan net.Conn

	// Non-memberlist/magic UDP packets
	udpCh chan *memberlist.Packet
	// Non-memberlist/magic TCP connections
	tcpCh chan net.Conn

	logger       *log.Logger
	wg           sync.WaitGroup
	tcpListeners []*net.TCPListener
	udpListeners []*net.UDPConn
	shutdown     int32
}

// NewNetTransport returns a net transport with the given configuration. On
// success all the network listeners will be created and listening.
func NewNetTransport(config *Config) (*NetTransport, error) {
	// If we reject the empty list outright we can assume that there's at
	// least one listener of each type later during operation.
	if len(config.BindAddrs) == 0 {
		return nil, fmt.Errorf("at least one bind address is required")
	}

	// Build out the new transport.
	t := NetTransport{
		config: config,
		pools:  make(map[uint8]*poolTransport, 1),
		muxed:  make(map[uint16]chan net.Conn),
		udpCh:  make(chan *memberlist.Packet),
		tcpCh:  make(chan net.Conn),
		logger: config.Logger,
	}

	err := t.initListeners(config)
	if err != nil {
		return nil, err
	}

	// ok = true
	return &t, nil
}

// Start starts listening on all bind addresses tcp and udp
func (t *NetTransport) Start() {
	// Fire them up now that we've been able to create them all.
	for i := 0; i < len(t.config.BindAddrs); i++ {
		t.wg.Add(2)
		go t.tcpListen(t.tcpListeners[i])
		go t.udpListen(t.udpListeners[i])
	}
}

// RegisterPool registers a new gossip pool returning a new pool
// transport that implements memberlist.Transport
func (t *NetTransport) RegisterPool(id uint8) memberlist.Transport {
	ptrans := newPoolTransport(id, t)
	t.pools[id] = ptrans
	return ptrans
}

// RegisterListener registers a new muxed listener
func (t *NetTransport) RegisterListener(id uint16) (<-chan net.Conn, error) {
	if _, ok := t.muxed[id]; ok {
		return nil, fmt.Errorf("listener id taken: %d", id)
	}

	// Check for memberlist magic
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, id)
	if buf[0] == memlistMagic {
		return nil, fmt.Errorf("listener id taken: %d", id)
	}

	ch := make(chan net.Conn)
	t.muxed[id] = ch
	return ch, nil
}

func (t *NetTransport) initListeners(config *Config) error {
	// Build all the TCP and UDP listeners.
	port := config.BindPort
	for _, addr := range config.BindAddrs {
		ip := net.ParseIP(addr)

		tcpAddr := &net.TCPAddr{IP: ip, Port: port}
		tcpLn, err := net.ListenTCP("tcp", tcpAddr)
		if err != nil {
			return fmt.Errorf("Failed to start TCP listener on %q port %d: %v", addr, port, err)
		}
		t.tcpListeners = append(t.tcpListeners, tcpLn)

		// If the config port given was zero, use the first TCP listener
		// to pick an available port and then apply that to everything
		// else.
		if port == 0 {
			port = tcpLn.Addr().(*net.TCPAddr).Port
		}

		udpAddr := &net.UDPAddr{IP: ip, Port: port}
		udpLn, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			return fmt.Errorf("Failed to start UDP listener on %q port %d: %v", addr, port, err)
		}
		if err := setUDPRecvBuf(udpLn); err != nil {
			return fmt.Errorf("Failed to resize UDP buffer: %v", err)
		}
		t.udpListeners = append(t.udpListeners, udpLn)
	}
	return nil
}

// GetAutoBindPort returns the bind port that was automatically given by the
// kernel, if a bind port of 0 was given.
func (t *NetTransport) GetAutoBindPort() int {
	// We made sure there's at least one TCP listener, and that one's
	// port was applied to all the others for the dynamic bind case.
	return t.tcpListeners[0].Addr().(*net.TCPAddr).Port
}

// FinalAdvertiseAddr implements memberlist.Transport. See Transport.
func (t *NetTransport) FinalAdvertiseAddr(ip string, port int) (net.IP, int, error) {
	var advertiseAddr net.IP
	var advertisePort int
	if ip != "" {
		// If they've supplied an address, use that.
		advertiseAddr = net.ParseIP(ip)
		if advertiseAddr == nil {
			return nil, 0, fmt.Errorf("Failed to parse advertise address %q", ip)
		}

		// Ensure IPv4 conversion if necessary.
		if ip4 := advertiseAddr.To4(); ip4 != nil {
			advertiseAddr = ip4
		}
		advertisePort = port
	} else {
		if t.config.BindAddrs[0] == "0.0.0.0" {
			// Otherwise, if we're not bound to a specific IP, let's
			// use a suitable private IP address.
			var err error
			ip, err = sockaddr.GetPrivateIP()
			if err != nil {
				return nil, 0, fmt.Errorf("failed to get interface addresses: %v", err)
			}
			if ip == "" {
				return nil, 0, fmt.Errorf("no private IP address found, and explicit IP not provided")
			}

			advertiseAddr = net.ParseIP(ip)
			if advertiseAddr == nil {
				return nil, 0, fmt.Errorf("failed to parse advertise address: %q", ip)
			}
		} else {
			// Use the IP that we're bound to, based on the first
			// TCP listener, which we already ensure is there.
			advertiseAddr = t.tcpListeners[0].Addr().(*net.TCPAddr).IP
		}

		// Use the port we are bound to.
		advertisePort = t.GetAutoBindPort()
	}

	return advertiseAddr, advertisePort, nil
}

// // WriteTo Satisfies memberlist.Transport. UDP write. See Transport.
// func (t *NetTransport) WriteTo(b []byte, addr string) (time.Time, error) {
// 	udpAddr, err := net.ResolveUDPAddr("udp", addr)
// 	if err != nil {
// 		return time.Time{}, err
// 	}

// 	// We made sure there's at least one UDP listener, so just use the
// 	// packet sending interface on the first one. Take the time after the
// 	// write call comes back, which will underestimate the time a little,
// 	// but help account for any delays before the write occurs.
// 	_, err = t.udpListeners[0].WriteTo(append([]byte{transUDPMagic}, b...), udpAddr)
// 	return time.Now(), err
// }

// // PacketCh implements memberlist.Transport. See Transport.
// func (t *NetTransport) PacketCh() <-chan *memberlist.Packet {
// 	return t.packetCh
// }

// // DialTimeout satisfies memberlist.Transport used for memberlist mux.
// func (t *NetTransport) DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
// 	dialer := net.Dialer{Timeout: timeout}
// 	conn, err := dialer.Dial("tcp", addr)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Write memlist magic
// 	if _, err = conn.Write([]byte{transTCPMagic}); err != nil {
// 		conn.Close()
// 		return nil, err
// 	}
// 	return conn, nil
// }

// // StreamCh satisfies memberlist.Transport
// func (t *NetTransport) StreamCh() <-chan net.Conn {
// 	return t.memlistCh
// }

// TCPCh returns a channel of all non-magic connections
func (t *NetTransport) TCPCh() <-chan net.Conn {
	return t.tcpCh
}

// UDPCh returns a channel of all non-magic connections
func (t *NetTransport) UDPCh() <-chan *memberlist.Packet {
	return t.udpCh
}

// Shutdown satisfies memberlist.Transport. See Transport.
func (t *NetTransport) Shutdown() error {
	if swapped := atomic.CompareAndSwapInt32(&t.shutdown, 0, 1); !swapped {
		return errors.New("transport already shutdown")
	}
	// atomic.StoreInt32(&t.shutdown, 1)

	// Rip through all the connections and shut them down.
	for _, conn := range t.tcpListeners {
		_ = conn.Close()
	}
	for _, conn := range t.udpListeners {
		_ = conn.Close()
	}

	// Block until all the listener threads have died.
	t.wg.Wait()

	// Close all muxed listeners
	for _, v := range t.muxed {
		close(v)
	}

	// Close non-muxed tcp and udp listener
	close(t.tcpCh)
	close(t.udpCh)

	// Shutdown pool channels
	for _, p := range t.pools {
		p.close()
	}

	return nil
}

// tcpListen is a long running goroutine that accepts incoming TCP connections
// and hands them off to the stream channel.
func (t *NetTransport) tcpListen(tcpLn *net.TCPListener) {
	defer t.wg.Done()

	// baseDelay is the initial delay after an AcceptTCP() error before attempting again
	const baseDelay = 5 * time.Millisecond
	// maxDelay is the maximum delay after an AcceptTCP() error before attempting again.
	// In the case that tcpListen() is error-looping, it will delay the shutdown check.
	// Therefore, changes to maxDelay may have an effect on the latency of shutdown.
	const maxDelay = 1 * time.Second

	var loopDelay time.Duration
	for {
		conn, err := tcpLn.AcceptTCP()

		if err != nil {
			if s := atomic.LoadInt32(&t.shutdown); s == 1 {
				break
			}

			if loopDelay == 0 {
				loopDelay = baseDelay
			} else {
				loopDelay *= 2
			}

			if loopDelay > maxDelay {
				loopDelay = maxDelay
			}

			t.logger.Printf("[ERR] Failed accepting TCP connection: %v", err)
			time.Sleep(loopDelay)
			continue
		}
		// No error, reset loop delay
		loopDelay = 0

		// Read magic in-order to mux
		magic := make([]byte, 2)
		if _, err = conn.Read(magic); err != nil {
			t.logger.Printf("[ERR] Failed reading magic: %v", err)
			conn.Close()
			continue
		}

		switch magic[0] {

		case memlistMagic:
			pool, ok := t.pools[magic[1]]
			if !ok {
				t.logger.Printf("[ERR] Gossip pool not found: %d", magic[1])
				conn.Close()
				continue
			}
			pool.memlistCh <- conn

		default:
			// User defined mux
			mgc := binary.BigEndian.Uint16(magic)
			if ch, ok := t.muxed[mgc]; ok {
				ch <- conn
				return
			}
			// No magic
			t.tcpCh <- &defaultNoMuxConn{first2: magic, Conn: conn}

		}

	}
}

// udpListen is a long running goroutine that accepts incoming UDP packets and
// hands them off to the packet channel.
func (t *NetTransport) udpListen(udpLn *net.UDPConn) {
	defer t.wg.Done()
	for {
		// Do a blocking read into a fresh buffer. Grab a time stamp as
		// close as possible to the I/O.
		buf := make([]byte, udpPacketBufSize)
		n, addr, err := udpLn.ReadFrom(buf)
		ts := time.Now()
		if err != nil {
			if s := atomic.LoadInt32(&t.shutdown); s == 1 {
				break
			}

			t.logger.Printf("[ERR] memberlist: Error reading UDP packet: %v", err)
			continue
		}

		// Check the length - it needs to have at least two byte to be a
		// proper message.
		if n < 2 {
			t.logger.Printf("[ERR] memberlist: UDP packet too short (%d bytes) %s",
				len(buf), addr.String())
			continue
		}

		switch buf[0] {
		case memlistMagic:
			pool, ok := t.pools[buf[1]]
			if !ok {
				t.logger.Printf("[ERR] Gossip pool not found: %d", buf[1])
				continue
			}

			// Ingest the packet.
			metrics.IncrCounter([]string{"memberlist", "udp", "received"}, float32(n))
			pool.packetCh <- &memberlist.Packet{
				Buf:       buf[2:n],
				From:      addr,
				Timestamp: ts,
			}

		default:
			// No magic
			t.udpCh <- &memberlist.Packet{
				Buf:       buf[:n],
				From:      addr,
				Timestamp: ts,
			}

		}
	}
}

// setUDPRecvBuf is used to resize the UDP receive window. The function
// attempts to set the read buffer to `udpRecvBuf` but backs off until
// the read buffer can be set.
func setUDPRecvBuf(c *net.UDPConn) error {
	size := udpRecvBufSize
	var err error
	for size > 0 {
		if err = c.SetReadBuffer(size); err == nil {
			return nil
		}
		size = size / 2
	}
	return err
}
