// Package transport isolates bounded, ordered socket output from gameplay locks.
package transport

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"time"
	"wonderland-gonline/internal/protocol"
)

const (
	MaxQueuedPackets = 4096
	MaxQueuedBytes   = 2 << 20
	WriteTimeout     = 10 * time.Second
	initialQueueSize = 16
)

var ErrBackpressure = errors.New("outgoing connection queue limit exceeded")

// Receipt reports the actual socket result, independently of queue acceptance.
// Only a complete successful write can authorize a durable delivery marker.
type Receipt struct {
	done   chan struct{}
	err    error
	notify func() // Optional nonblocking completion signal; never gameplay work.
}

func (r *Receipt) Result() (bool, error) {
	select {
	case <-r.done:
		return true, r.err
	default:
		return false, nil
	}
}
func (r *Receipt) finish(err error) {
	r.err = err
	close(r.done)
	if r.notify != nil {
		r.notify()
	}
}

type message struct {
	frame    []byte
	receipt  *Receipt
	deadline time.Time
}

type limits struct {
	packets, bytes int
	timeout        time.Duration
}

// Conn owns one writer. Network operations, tracing and completion observation
// never run under its queue mutex. Closing never waits for that writer.
type Conn struct {
	conn                         net.Conn
	mu                           sync.Mutex
	wake                         *sync.Cond
	queue                        []message
	head, size                   int
	pendingPackets, pendingBytes int // Includes the in-flight frame.
	terminal                     error
	draining                     bool
	limits                       limits
	done                         chan struct{}
	closeOnce                    sync.Once
	observe                      func([]byte, error) // Encoded frame; valid only during the call.
}

func New(conn net.Conn, observe func([]byte, error)) *Conn {
	return newConn(conn, limits{MaxQueuedPackets, MaxQueuedBytes, WriteTimeout}, observe)
}
func newConn(conn net.Conn, bound limits, observe func([]byte, error)) *Conn {
	c := &Conn{conn: conn, limits: bound, done: make(chan struct{}), observe: observe}
	c.wake = sync.NewCond(&c.mu)
	go c.run()
	return c
}

// Send takes an immutable encoded copy; nil means queued, not delivered.
func (c *Conn) Send(payload []byte) error {
	frame, err := protocol.Encode(payload)
	if err != nil {
		return err
	}
	return c.enqueue(message{frame: frame})
}

// SendTracked additionally reports actual write completion. The optional notify
// signal must be nonblocking and must not acquire server locks or perform SQL.
func (c *Conn) SendTracked(payload []byte, notify func()) (*Receipt, error) {
	frame, err := protocol.Encode(payload)
	if err != nil {
		return nil, err
	}
	r := &Receipt{done: make(chan struct{}), notify: notify}
	if err := c.enqueue(message{frame: frame, receipt: r}); err != nil {
		return nil, err
	}
	return r, nil
}

// Write also queues raw framed bytes, so using Conn as an io.Writer cannot
// accidentally reintroduce synchronous socket writes in a gameplay handler.
func (c *Conn) Write(frame []byte) (int, error) {
	if len(frame) == 0 {
		return 0, nil
	}
	if len(frame) > c.limits.bytes {
		c.mu.Lock()
		c.stopLocked(ErrBackpressure)
		c.mu.Unlock()
		c.closeSocket()
		return 0, ErrBackpressure
	}
	if err := c.enqueue(message{frame: bytes.Clone(frame)}); err != nil {
		return 0, err
	}
	return len(frame), nil
}
func (c *Conn) enqueue(m message) error {
	c.mu.Lock()
	if c.terminal != nil || c.draining {
		err := c.terminal
		if err == nil {
			err = net.ErrClosed
		}
		c.mu.Unlock()
		return err
	}
	if c.pendingPackets >= c.limits.packets || len(m.frame) > c.limits.bytes-c.pendingBytes {
		c.stopLocked(ErrBackpressure)
		c.mu.Unlock()
		c.closeSocket()
		return ErrBackpressure
	}
	if c.size == len(c.queue) {
		next := make([]message, min(c.limits.packets, max(initialQueueSize, 2*len(c.queue))))
		for i := 0; i < c.size; i++ {
			next[i] = c.queue[(c.head+i)%len(c.queue)]
		}
		c.queue, c.head = next, 0
	}
	m.deadline = time.Now().Add(c.limits.timeout)
	c.queue[(c.head+c.size)%len(c.queue)] = m
	c.size++
	c.pendingPackets++
	c.pendingBytes += len(m.frame)
	c.wake.Signal()
	c.mu.Unlock()
	return nil
}

func (c *Conn) Read(p []byte) (int, error) {
	c.mu.Lock()
	closing := c.draining || c.terminal != nil
	c.mu.Unlock()
	if closing {
		return 0, net.ErrClosed
	}
	return c.conn.Read(p)
}

func (c *Conn) LocalAddr() net.Addr                { return c.conn.LocalAddr() }
func (c *Conn) RemoteAddr() net.Addr               { return c.conn.RemoteAddr() }
func (c *Conn) SetDeadline(t time.Time) error      { return c.conn.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.conn.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.conn.SetWriteDeadline(t) }

// CloseAfterFlush rejects new output and lets existing frames finish under
// their original enqueue deadlines. It never waits or extends those deadlines.
func (c *Conn) CloseAfterFlush() {
	c.mu.Lock()
	c.draining = true
	c.wake.Broadcast()
	c.mu.Unlock()
}
func (c *Conn) Draining() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.draining }
func (c *Conn) Accepting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.terminal == nil && !c.draining
}
func (c *Conn) Done() <-chan struct{} { return c.done }
func (c *Conn) Close() error {
	c.mu.Lock()
	c.stopLocked(net.ErrClosed)
	c.mu.Unlock()
	c.closeSocket()
	return nil
}
func (c *Conn) closeSocket() { c.closeOnce.Do(func() { c.conn.Close() }) }
func (c *Conn) stopLocked(err error) {
	if c.terminal != nil {
		return
	}
	c.terminal = err
	for i := 0; i < c.size; i++ {
		m := c.queue[(c.head+i)%len(c.queue)]
		c.pendingPackets--
		c.pendingBytes -= len(m.frame)
		if m.receipt != nil {
			m.receipt.finish(err)
		}
	}
	c.queue, c.head, c.size = nil, 0, 0
	c.wake.Broadcast()
}

func (c *Conn) run() {
	defer close(c.done)
	defer c.closeSocket()
	for {
		c.mu.Lock()
		for c.size == 0 && c.terminal == nil && !c.draining {
			c.wake.Wait()
		}
		if c.terminal != nil || c.size == 0 && c.draining {
			c.stopLocked(net.ErrClosed)
			err := c.terminal
			c.mu.Unlock()
			if errors.Is(err, ErrBackpressure) && c.observe != nil {
				c.observe(nil, err)
			}
			return
		}
		m := c.queue[c.head]
		c.queue[c.head] = message{}
		c.head = (c.head + 1) % len(c.queue)
		c.size--
		c.mu.Unlock()

		err := c.conn.SetWriteDeadline(m.deadline)
		if err == nil {
			for raw := m.frame; len(raw) > 0; {
				n, writeErr := c.conn.Write(raw)
				if writeErr != nil {
					err = writeErr
					break
				}
				if n <= 0 || n > len(raw) {
					err = io.ErrShortWrite
					break
				}
				raw = raw[n:]
			}
		}
		if m.receipt != nil {
			m.receipt.finish(err)
		}
		c.mu.Lock()
		c.pendingPackets--
		c.pendingBytes -= len(m.frame)
		if err != nil {
			c.stopLocked(err)
		}
		reportErr := err
		if err != nil {
			reportErr = c.terminal
		}
		c.mu.Unlock()
		if c.observe != nil {
			c.observe(m.frame, reportErr)
		}
		if err != nil {
			return
		}
	}
}
