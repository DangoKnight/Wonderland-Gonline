//go:build js && wasm

// Browser build of the client. The page stores the asset packs as Blobs
// (IndexedDB, downloaded once per browser) and sets window.wloPacks to an
// object of name -> Blob, plus window.wloReadBlob(name, offset, length) which
// returns a Promise<ArrayBuffer>. Packs are read on demand, never loaded whole.
package main

import (
	"fmt"
	"io"
	"log"
	"sort"
	"syscall/js"

	"wonderland-gonline/client/wlo/app"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/wsdial"
)

// localServerINI is used when no SERVER.INI is available: one local server.
var localServerINI = []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")

func main() {
	if err := run(); err != nil {
		log.Printf("wonderland web client: %v", err)
		js.Global().Get("console").Call("error", "wonderland web client: "+err.Error())
		return
	}
	// Keep the Go runtime alive: returning from main ends the program and
	// Ebiten's browser callbacks then report "Go program has already exited".
	select {}
}

// blobReader reads one pack through the page's JS reader. Each call waits
// for the Promise; the Go goroutine blocks while the browser keeps running.
type blobReader struct {
	name string
	size int64
}

func (b blobReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= b.size {
		return 0, io.EOF
	}
	n := int64(len(p))
	if off+n > b.size {
		n = b.size - off
	}
	if n == 0 {
		return 0, nil
	}
	done := make(chan []byte, 1)
	failed := make(chan error, 1)
	promise := js.Global().Call("wloReadBlob", b.name, float64(off), float64(n))
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		arr := args[0]
		buf := make([]byte, arr.Get("byteLength").Int())
		js.CopyBytesToGo(buf, js.Global().Get("Uint8Array").New(arr))
		done <- buf
		return nil
	})
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any {
		failed <- fmt.Errorf("read %s at %d: %s", b.name, off, args[0].Call("toString").String())
		return nil
	})
	promise.Call("then", ok, bad)
	select {
	case buf := <-done:
		copied := copy(p, buf)
		if copied < len(p) {
			return copied, io.ErrUnexpectedEOF
		}
		return copied, nil
	case err := <-failed:
		return 0, err
	}
}

func run() error {
	packs := js.Global().Get("wloPacks")
	if packs.IsUndefined() || packs.IsNull() {
		return fmt.Errorf("window.wloPacks is not set")
	}
	keys := js.Global().Get("Object").Call("keys", packs)
	names := make([]string, 0, keys.Length())
	for i := 0; i < keys.Length(); i++ {
		names = append(names, keys.Index(i).String())
	}
	sort.Strings(names)
	var sources []clientfs.ZipSource
	for _, name := range names {
		blob := packs.Get(name)
		size := int64(blob.Get("size").Float())
		sources = append(sources, clientfs.ZipSource{R: blobReader{name: name, size: size}, Size: size})
	}
	root, _, err := clientfs.MountPacks("client-assets", sources)
	if err != nil {
		return err
	}
	c, err := app.New(app.Options{
		Renderer: render.Auto,
		Root:     root,
		// The browser build has no jma archive to size the layout from; use the level the desktop install resolves to.
		ContentLevel: 5,
		FallbackINI:  localServerINI,
		Dial:         wsdial.Dial,
	})
	if err != nil {
		return err
	}
	return app.Run(c)
}
