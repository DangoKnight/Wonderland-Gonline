//go:build js && wasm

// Package wsdial gives the browser client a net.Dial replacement. Each
// connection is a WebSocket to this page's /ws endpoint; the gateway there
// relays the bytes to the requested loopback TCP port.
package wsdial

import (
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"syscall/js"
	"time"
)

type conn struct {
	ws        js.Value
	in        chan []byte
	closed    chan struct{}
	once      sync.Once
	mu        sync.Mutex
	pending   []byte
	remote    string
	callbacks []js.Func
}

const (
	webSocketOpen          = 1
	webSocketFrameLimit    = 1 << 20
	webSocketBufferedLimit = 2 << 20
)

var socketEvents = [...]string{"open", "message", "close", "error"}

// Dial opens a WebSocket to /ws?target=address and waits for it to open.
func Dial(network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, errors.New("wsdial: only tcp is supported")
	}
	loc := js.Global().Get("location")
	scheme := "ws:"
	if loc.Get("protocol").String() == "https:" {
		scheme = "wss:"
	}
	target := scheme + "//" + loc.Get("host").String() + "/ws?target=" + url.QueryEscape(address)
	ws := js.Global().Get("WebSocket").New(target)
	ws.Set("binaryType", "arraybuffer")

	c := &conn{ws: ws, in: make(chan []byte, 256), closed: make(chan struct{}), remote: address}
	opened := make(chan error, 1)
	signal := func(err error) {
		select {
		case opened <- err:
		default:
		}
	}
	onOpen := js.FuncOf(func(js.Value, []js.Value) any { signal(nil); return nil })
	onMessage := js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		if !data.InstanceOf(js.Global().Get("ArrayBuffer")) || data.Get("byteLength").Int() > webSocketFrameLimit {
			c.Close()
			return nil
		}
		b := make([]byte, data.Get("byteLength").Int())
		js.CopyBytesToGo(b, js.Global().Get("Uint8Array").New(data))
		select {
		case c.in <- b:
		case <-c.closed:
		default:
			// Never block the browser event loop on a stalled receiver.
			c.Close()
		}
		return nil
	})
	onClose := js.FuncOf(func(js.Value, []js.Value) any {
		signal(errors.New("wsdial: connection closed"))
		c.shutdown()
		return nil
	})
	onError := js.FuncOf(func(js.Value, []js.Value) any {
		signal(errors.New("wsdial: websocket error"))
		c.Close()
		return nil
	})
	c.callbacks = []js.Func{onOpen, onMessage, onClose, onError}
	ws.Call("addEventListener", "open", onOpen)
	ws.Call("addEventListener", "message", onMessage)
	ws.Call("addEventListener", "close", onClose)
	ws.Call("addEventListener", "error", onError)

	select {
	case err := <-opened:
		if err != nil {
			c.Close()
			return nil, err
		}
	case <-time.After(10 * time.Second):
		c.Close()
		return nil, errors.New("wsdial: connect timeout")
	}
	return c, nil
}

func (c *conn) shutdown() {
	c.once.Do(func() {
		close(c.closed)
		for i, callback := range c.callbacks {
			c.ws.Call("removeEventListener", socketEvents[i], callback)
			callback.Release()
		}
		c.callbacks = nil
	})
}

func (c *conn) take(p []byte, b []byte) int {
	n := copy(p, b)
	if n < len(b) {
		c.mu.Lock()
		c.pending = append(c.pending, b[n:]...)
		c.mu.Unlock()
	}
	return n
}

func (c *conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.mu.Lock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.mu.Unlock()
		return n, nil
	}
	c.mu.Unlock()
	select {
	case b := <-c.in:
		return c.take(p, b), nil
	case <-c.closed:
		select {
		case b := <-c.in:
			return c.take(p, b), nil
		default:
			return 0, io.EOF
		}
	}
}

func (c *conn) Write(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
	}
	if c.ws.Get("readyState").Int() != webSocketOpen {
		return 0, net.ErrClosed
	}
	if len(p) > webSocketBufferedLimit || c.ws.Get("bufferedAmount").Int() > webSocketBufferedLimit-len(p) {
		c.Close()
		return 0, errors.New("wsdial: outgoing buffer exceeded")
	}
	arr := js.Global().Get("Uint8Array").New(len(p))
	js.CopyBytesToJS(arr, p)
	c.ws.Call("send", arr)
	return len(p), nil
}

func (c *conn) Close() error {
	c.shutdown()
	c.ws.Call("close")
	return nil
}

func (c *conn) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *conn) RemoteAddr() net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", c.remote)
	if a == nil {
		return &net.TCPAddr{}
	}
	return a
}

func (c *conn) SetDeadline(time.Time) error      { return nil }
func (c *conn) SetReadDeadline(time.Time) error  { return nil }
func (c *conn) SetWriteDeadline(time.Time) error { return nil }
