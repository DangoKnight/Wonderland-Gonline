//go:build js && wasm

// Browser build of the client. The page stores the asset packs as Blobs
// (IndexedDB, downloaded once per browser) and sets window.wloPacks to an
// object of name -> Blob, plus window.wloReadBlob(name, offset, length) which
// returns a Promise<ArrayBuffer>. Packs are read on demand, never loaded whole.
package main

import (
	"fmt"
	"image"
	"io"
	"log"
	"sort"
	"sync"
	"syscall/js"

	"wonderland-gonline/client/wlo/app"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/client/wlo/world"
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

// blobReader reads one pack through the page's JS reader. Reads are served
// from a block cache: each miss fetches one aligned block, so the many small
// reads a ZIP directory and image decoder make do not each cross into JS.
type blobReader struct {
	name string
	size int64
}

const (
	blockSize   = 1 << 20 // bytes per cached block
	blockBudget = 96      // blocks kept in memory (about 96 MiB)
)

type blockKey struct {
	name string
	idx  int64
}

var blocks = struct {
	sync.Mutex
	data  map[blockKey][]byte
	order []blockKey
}{data: map[blockKey][]byte{}}

// fetchBlock asks the page for [start, start+n) of a pack and waits for it.
func fetchBlock(name string, start, n int64) ([]byte, error) {
	done := make(chan []byte, 1)
	failed := make(chan error, 1)
	promise := js.Global().Call("wloReadBlob", name, float64(start), float64(n))
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		arr := args[0]
		buf := make([]byte, arr.Get("byteLength").Int())
		js.CopyBytesToGo(buf, js.Global().Get("Uint8Array").New(arr))
		done <- buf
		return nil
	})
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any {
		failed <- fmt.Errorf("read %s at %d: %s", name, start, args[0].Call("toString").String())
		return nil
	})
	promise.Call("then", ok, bad)
	defer ok.Release()
	defer bad.Release()
	select {
	case buf := <-done:
		return buf, nil
	case err := <-failed:
		return nil, err
	}
}

// block returns block idx of a pack, from memory when present.
func block(name string, size, idx int64) ([]byte, error) {
	key := blockKey{name, idx}
	blocks.Lock()
	if data, ok := blocks.data[key]; ok {
		blocks.Unlock()
		return data, nil
	}
	blocks.Unlock()
	start := idx * blockSize
	n := int64(blockSize)
	if start+n > size {
		n = size - start
	}
	data, err := fetchBlock(name, start, n)
	if err != nil {
		return nil, err
	}
	blocks.Lock()
	defer blocks.Unlock()
	if _, ok := blocks.data[key]; !ok {
		blocks.data[key] = data
		blocks.order = append(blocks.order, key)
		for len(blocks.order) > blockBudget {
			delete(blocks.data, blocks.order[0])
			blocks.order = blocks.order[1:]
		}
	}
	return data, nil
}

func (b blobReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= b.size {
		return 0, io.EOF
	}
	want := int64(len(p))
	if off+want > b.size {
		want = b.size - off
	}
	read := int64(0)
	for read < want {
		pos := off + read
		idx := pos / blockSize
		data, err := block(b.name, b.size, idx)
		if err != nil {
			return int(read), err
		}
		within := pos - idx*blockSize
		if within >= int64(len(data)) {
			return int(read), io.ErrUnexpectedEOF
		}
		read += int64(copy(p[read:want], data[within:]))
	}
	if read < int64(len(p)) {
		return int(read), io.EOF
	}
	return int(read), nil
}

// decodeViaBrowser decodes a picture with the browser's image decoder (which can
// use the GPU or a hardware decoder), instead of the WebAssembly PNG decoder.
// The page returns RGBA pixels; they are copied into an NRGBA image.
func decodeViaBrowser(raw []byte) (image.Image, error) {
	arr := js.Global().Get("Uint8Array").New(len(raw))
	js.CopyBytesToJS(arr, raw)
	type result struct {
		img image.Image
		err error
	}
	done := make(chan result, 1)
	promise := js.Global().Call("wloDecodeImage", arr)
	ok := js.FuncOf(func(_ js.Value, args []js.Value) any {
		res := args[0]
		w, h := res.Get("width").Int(), res.Get("height").Int()
		data := res.Get("data")
		pix := make([]byte, w*h*4)
		js.CopyBytesToGo(pix, data)
		done <- result{img: &image.NRGBA{Pix: pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}}
		return nil
	})
	bad := js.FuncOf(func(_ js.Value, args []js.Value) any {
		done <- result{err: fmt.Errorf("browser image decode: %s", args[0].Call("toString").String())}
		return nil
	})
	promise.Call("then", ok, bad)
	defer ok.Release()
	defer bad.Release()
	r := <-done
	return r.img, r.err
}

func run() error {
	// imagedecode.Hook = decodeViaBrowser // disabled: slower and corrupted rendering (see notes)
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
	// Parse the terrain export now, while the login screens load, instead of
	// stalling on the first map change.
	if err := world.WarmTerrain(login.NewAssets(root)); err != nil {
		log.Printf("terrain warm-up: %v", err)
	}
	c, err := app.New(app.Options{
		Renderer:    render.Auto,
		Root:        root,
		FallbackINI: localServerINI,
		Dial:        wsdial.Dial,
	})
	if err != nil {
		return err
	}
	clientRef = c
	installAgentAPI()
	go func() {
		if err := app.Run(c); err != nil {
			log.Printf("wonderland web client: %v", err)
			js.Global().Get("console").Call("error", "wonderland web client: "+err.Error())
		}
	}()
	return nil
}
