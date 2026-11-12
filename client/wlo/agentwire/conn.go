// Package agentwire provides bounded newline-delimited JSON over native TCP.
// It has no HTTP, WebSocket, browser or rendering dependency.
package agentwire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
)

const DefaultMessageLimit = 1 << 20

type Conn struct {
	net.Conn
	reader *bufio.Reader
	limit  int64
}

func New(conn net.Conn) *Conn {
	return &Conn{Conn: conn, reader: bufio.NewReader(conn), limit: DefaultMessageLimit}
}
func (c *Conn) SetReadLimit(limit int64) { c.limit = limit }
func (c *Conn) ReadJSON(value any) error {
	var message []byte
	for {
		part, err := c.reader.ReadSlice('\n')
		if int64(len(message)+len(part)) > c.limit {
			return errors.New("native API message exceeds limit")
		}
		message = append(message, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(bytes.TrimSpace(message), value)
	}
}
func (c *Conn) WriteJSON(value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	for len(b) > 0 {
		n, err := c.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
