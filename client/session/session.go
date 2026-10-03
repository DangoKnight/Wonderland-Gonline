// Package session is the client side of the Wonderland login protocol.
package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"wonderland-go/internal/protocol"
)

// Ports used by the original client: the status socket is set to 0x1910 in
// aLogin (FUN_003ff82c); login uses the game port.
const (
	LoginPort  = 6414
	StatusPort = 6416
)

// Status fetches server signals from a region server's status port, as aLogin
// does when a region is selected: an unframed reply of a three-byte header
// followed by (u16 server ID, u8 signal) records, read until the server
// closes the connection.
func Status(host string, timeout time.Duration) (map[int]int, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(StatusPort)), timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	b, err := io.ReadAll(io.LimitReader(conn, 4096))
	if err != nil && !errors.Is(err, io.EOF) && len(b) == 0 {
		return nil, err
	}
	return ParseStatus(b)
}

// ParseStatus decodes a status reply.
func ParseStatus(b []byte) (map[int]int, error) {
	if len(b) < 3 || b[0] != 0xc9 {
		return nil, fmt.Errorf("status: unexpected reply % x", b)
	}
	out := map[int]int{}
	for r := b[3:]; len(r) >= 3; r = r[3:] {
		out[int(r[0])|int(r[1])<<8] = int(r[2])
	}
	return out, nil
}

// Conn is a framed connection to a login server.
type Conn struct {
	conn    net.Conn
	packets chan []byte
	mu      sync.Mutex
	err     error
}

// Dial connects and sends the opening action 0, which the server answers with
// its name and channel list.
func Dial(host string, timeout time.Duration) (*Conn, error) {
	nc, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(LoginPort)), timeout)
	if err != nil {
		return nil, err
	}
	c := &Conn{conn: nc, packets: make(chan []byte, 64)}
	go c.read()
	if err := c.Send([]byte{0, 0}); err != nil {
		nc.Close()
		return nil, err
	}
	return c, nil
}

func (c *Conn) read() {
	defer close(c.packets)
	for {
		p, err := protocol.Read(c.conn)
		if err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		c.packets <- p
	}
}

// Packets delivers received payloads; it is closed when the connection ends.
func (c *Conn) Packets() <-chan []byte { return c.packets }

// Err is the error that ended the connection, if any.
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Conn) Send(p []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return protocol.Write(c.conn, p)
}

func (c *Conn) Close() error { return c.conn.Close() }

// Login sends 63/4 with the account and password as length-prefixed strings.
// The original may prefix a client version word, which servers treat as
// optional; it is omitted until traced.
func (c *Conn) Login(account, password []byte) error {
	if len(account) > 255 || len(password) > 255 {
		return errors.New("login: field too long")
	}
	var b bytes.Buffer
	b.Write([]byte{63, 4, byte(len(account))})
	b.Write(account)
	b.WriteByte(byte(len(password)))
	b.Write(password)
	return c.Send(b.Bytes())
}

// LoginResult classifies the server's reply to a login.
type LoginResult int

const (
	LoginPending LoginResult = iota
	LoginOK
	LoginRejected  // 63/2 then 1/6: wrong account or password
	LoginDuplicate // 63/2 then 0/19: account already online
)

// Classify inspects one packet received after Login. A bare 63/2 is the
// rejection prefix; 63/2 with a user ID accepts the login.
func Classify(p []byte, sawReject bool) (LoginResult, bool) {
	switch {
	case len(p) >= 6 && p[0] == 63 && p[1] == 2:
		return LoginOK, false
	case len(p) == 2 && p[0] == 63 && p[1] == 2:
		return LoginPending, true
	case sawReject && len(p) >= 2 && p[0] == 1 && p[1] == 6:
		return LoginRejected, false
	case sawReject && len(p) >= 2 && p[0] == 0 && p[1] == 19:
		return LoginDuplicate, false
	}
	return LoginPending, sawReject
}
