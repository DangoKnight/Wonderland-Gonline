//go:build js && wasm

package wsdial

import (
	"net"
	"syscall/js"
	"testing"
)

func browserSocketFixture(t *testing.T) *conn {
	t.Helper()
	global := js.Global()
	oldSocket, oldLocation := global.Get("WebSocket"), global.Get("location")
	global.Call("eval", `globalThis.WebSocket=class {
 constructor(){this.readyState=0;this.bufferedAmount=0;this.handlers={};setTimeout(()=>{this.readyState=1;this.emit('open',{});},0);}
 addEventListener(name,callback){this.handlers[name]=callback;}
 removeEventListener(name){delete this.handlers[name];}
 emit(name,event){if(this.handlers[name])this.handlers[name](event);}
 send(data){this.sent=data;}
 close(){this.readyState=3;}
};`)
	global.Set("location", js.ValueOf(map[string]any{"protocol": "http:", "host": "localhost:8090"}))
	t.Cleanup(func() { global.Set("WebSocket", oldSocket); global.Set("location", oldLocation) })
	raw, err := Dial("tcp", "127.0.0.1:6414")
	if err != nil {
		t.Fatal(err)
	}
	c := raw.(*conn)
	t.Cleanup(func() { c.Close() })
	return c
}

func TestBrowserSocketReadWriteAndCallbackCleanup(t *testing.T) {
	c := browserSocketFixture(t)
	if n, err := c.Write([]byte{1, 2, 3}); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	if c.ws.Get("sent").Get("byteLength").Int() != 3 {
		t.Fatal("binary write lost bytes")
	}
	data := js.Global().Get("Uint8Array").New(3)
	js.CopyBytesToJS(data, []byte{4, 5, 6})
	c.ws.Call("emit", "message", js.ValueOf(map[string]any{"data": data.Get("buffer")}))
	buf := make([]byte, 3)
	if n, err := c.Read(buf); err != nil || n != 3 || buf[2] != 6 {
		t.Fatal(buf, n, err)
	}
	c.Close()
	c.Close()
	if len(c.callbacks) != 0 || js.Global().Get("Object").Call("keys", c.ws.Get("handlers")).Length() != 0 {
		t.Fatal("closed connection retains browser callbacks")
	}
	if _, err := c.Write([]byte{1}); err != net.ErrClosed {
		t.Fatal("closed write did not fail", err)
	}
}

func TestBrowserSocketOverflowDoesNotBlockEventLoop(t *testing.T) {
	c := browserSocketFixture(t)
	for i := 0; i < cap(c.in); i++ {
		c.in <- []byte{1}
	}
	data := js.Global().Get("Uint8Array").New(1)
	c.ws.Call("emit", "message", js.ValueOf(map[string]any{"data": data.Get("buffer")}))
	select {
	case <-c.closed:
	default:
		t.Fatal("overfull receiver was not disconnected")
	}
}

func TestBrowserSocketBoundsOutgoingQueue(t *testing.T) {
	c := browserSocketFixture(t)
	c.ws.Set("bufferedAmount", webSocketBufferedLimit)
	if _, err := c.Write([]byte{1}); err == nil {
		t.Fatal("unbounded browser write accepted")
	}
	select {
	case <-c.closed:
	default:
		t.Fatal("overfull sender was not disconnected")
	}
}
