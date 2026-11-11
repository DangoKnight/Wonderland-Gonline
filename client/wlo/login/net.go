// Package login ports the login phase of aLogin.exe: the server list
// (TSe_SelectServer), the account form (TSe_IDPassored), the status and
// login sockets, and the packets between them. Addresses are from the WLRI
// build decompile; docs/CLIENT.md lists the traced functions.
package login

import (
	"encoding/binary"
	"log"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"wonderland-gonline/client/debuglog"
	"wonderland-gonline/internal/protocol"
)

// Ports from the form design (ClientSocket1) and FUN_003ff82c.
const (
	LoginPort  = 6414
	StatusPort = 0x1910
)

// statusSettle is the pause after closing an open status socket
// (Sleep(0x32) in FUN_003ff82c).
const statusSettle = 50 * time.Millisecond
const connectionDialTimeout = 10 * time.Second

// EventKind tells the frame loop what a socket did. The original receives
// these as TClientSocket events on the main thread.
type EventKind int

const (
	StatusData    EventKind = 1 // ClientSocket3Read
	Connected     EventKind = 2 // ClientSocket1Connect
	Packet        EventKind = 3 // a frame queued by ClientSocket1Read
	Disconnected  EventKind = 4 // ClientSocket1Disconnect
	ConnectFailed EventKind = 5 // ClientSocket1Error
)

type Event struct {
	Kind EventKind
	Data []byte
}

// Net is the pair of client sockets. Events arrive on a channel the frame
// loop drains, so handlers run on the game thread as in the original.
type Net struct {
	Dial   func(network, address string) (net.Conn, error)
	events chan Event
	mu     sync.Mutex
	status net.Conn
	login  net.Conn
	// observer records every packet that crosses the login socket; nil disables
	// recording (see client/debuglog).
	observer   func(direction byte, p []byte)
	diagnostic func(format string, args ...any)
	reframe    []byte // DAT_00828128 +8
	dialing    bool
	closed     bool
	pending    [][]byte
}

// SetObserver installs or clears the packet recorder. It is called from the
// game thread when session recording is toggled.
func (n *Net) SetObserver(observe func(direction byte, p []byte)) {
	n.mu.Lock()
	n.observer = observe
	n.mu.Unlock()
}

// SetDiagnostic supplies a session-owned diagnostic sink. It must be safe for
// concurrent calls from the connection reader and the UI thread.
func (n *Net) SetDiagnostic(report func(format string, args ...any)) {
	n.mu.Lock()
	n.diagnostic = report
	n.mu.Unlock()
}

func (n *Net) report(format string, args ...any) {
	n.mu.Lock()
	report := n.diagnostic
	n.mu.Unlock()
	if report != nil {
		report(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

// observe hands one packet to the recorder installed by SetObserver.
func (n *Net) observe(direction byte, p []byte) {
	n.mu.Lock()
	observe := n.observer
	n.mu.Unlock()
	if observe != nil {
		observe(direction, p)
	}
}

func NewNet() *Net {
	return &Net{Dial: (&net.Dialer{Timeout: connectionDialTimeout}).Dial, events: make(chan Event, 256)}
}

// Poll returns the events received since the last call.
func (n *Net) Poll() []Event {
	var out []Event
	for {
		select {
		case e := <-n.events:
			out = append(out, e)
		default:
			return out
		}
	}
}

func (n *Net) post(e Event) {
	select {
	case n.events <- e:
	default:
	}
}

// StatusActive reports whether ClientSocket3 is open.
func (n *Net) StatusActive() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status != nil
}

// CloseStatus is ClientSocket3's Close, followed by the original's pause.
func (n *Net) CloseStatus() {
	n.mu.Lock()
	c := n.status
	n.status = nil
	n.mu.Unlock()
	if c != nil {
		c.Close()
		time.Sleep(statusSettle)
	}
}

// QueryStatus opens ClientSocket3 to host. Every read is posted as
// StatusData; errors are dropped (ClientSocket3Error zeroes the code).
func (n *Net) QueryStatus(host string) {
	go func() {
		c, err := n.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(StatusPort)))
		if err != nil {
			return
		}
		n.mu.Lock()
		if n.closed {
			n.mu.Unlock()
			c.Close()
			return
		}
		n.status = c
		n.mu.Unlock()
		buf := make([]byte, 4096)
		for {
			k, err := c.Read(buf)
			if k > 0 {
				n.post(Event{StatusData, append([]byte(nil), buf[:k]...)})
			}
			if err != nil {
				break
			}
		}
		n.mu.Lock()
		if n.status == c {
			n.status = nil
		}
		n.mu.Unlock()
		c.Close()
	}()
}

// Connect opens ClientSocket1 to host.
func (n *Net) Connect(host string) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	n.dialing, n.pending = true, nil
	n.mu.Unlock()
	go func() {
		// A panic here would kill the process with no trace; record the stack in
		// the session transcript, then let it die as before.
		defer func() {
			if r := recover(); r != nil {
				n.report("panic in the connection reader: %v\n%s", r, debug.Stack())
				panic(r)
			}
		}()
		c, err := n.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(LoginPort)))
		n.mu.Lock()
		if n.closed {
			n.mu.Unlock()
			if c != nil {
				c.Close()
			}
			return
		}
		n.dialing = false
		pending := n.pending
		n.pending = nil
		if err == nil {
			n.login = c
			n.reframe = nil
		}
		n.mu.Unlock()
		if err != nil {
			n.post(Event{Kind: ConnectFailed})
			return
		}
		n.post(Event{Kind: Connected})
		for _, p := range pending {
			n.Send(p)
		}
		buf := make([]byte, 4096)
		for {
			k, err := c.Read(buf)
			if k > 0 {
				for _, p := range n.receive(buf[:k]) {
					n.observe(debuglog.In, p)
					n.post(Event{Packet, p})
				}
			}
			if err != nil {
				break
			}
		}
		n.mu.Lock()
		mine := n.login == c
		if mine {
			n.login = nil
		}
		n.mu.Unlock()
		if mine {
			n.post(Event{Kind: Disconnected})
		}
	}()
}

// Close is ClientSocket1's Close. The original's disconnect event fires
// only for a connection the server ends.
func (n *Net) Close() {
	n.mu.Lock()
	c := n.login
	n.login, n.dialing, n.pending = nil, false, nil
	n.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

// receive is ClientSocket1Read (0x498470): the data is decoded with the
// XOR key and appended to the buffer, and complete frames are cut from it.
// A buffer that does not start with the signature loses two bytes.
func (n *Net) receive(data []byte) [][]byte {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, b := range data {
		n.reframe = append(n.reframe, b^protocol.XOR)
	}
	var out [][]byte
	for len(n.reframe) >= protocol.FrameHeaderBytes {
		if binary.LittleEndian.Uint16(n.reframe) != protocol.Signature {
			n.reframe = n.reframe[2:]
			continue
		}
		size := int(binary.LittleEndian.Uint16(n.reframe[2:]))
		if size > len(n.reframe)-protocol.FrameHeaderBytes {
			break
		}
		end := protocol.FrameHeaderBytes + size
		out = append(out, append([]byte(nil), n.reframe[protocol.FrameHeaderBytes:end]...))
		n.reframe = n.reframe[end:]
	}
	return out
}

// Send is CY_AddSedQueue (0x4a3044): the payload is framed and sent. The
// original queues a send made right after opening the socket, such as
// action 0; it is sent once the connection completes.
func (n *Net) Send(p []byte) error {
	n.mu.Lock()
	c := n.login
	if c == nil && n.dialing {
		n.pending = append(n.pending, p)
	}
	n.mu.Unlock()
	if c == nil {
		return net.ErrClosed
	}
	c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := protocol.Write(c, p); err != nil {
		return err
	}
	n.observe(debuglog.Out, p)
	return nil
}

// Shutdown permanently disposes a removed session, including late dial results.
// Ordinary logout uses Close and can reconnect.
func (n *Net) Shutdown() {
	n.mu.Lock()
	n.closed = true
	login, status := n.login, n.status
	n.login, n.status = nil, nil
	n.pending = nil
	n.dialing = false
	n.mu.Unlock()
	if login != nil {
		login.Close()
	}
	if status != nil {
		status.Close()
	}
}

// ServerAddress identifies the actual connected endpoint, not a display name.
func (n *Net) ServerAddress() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.login == nil {
		return ""
	}
	a := n.login.RemoteAddr()
	return a.Network() + ":" + a.String()
}
