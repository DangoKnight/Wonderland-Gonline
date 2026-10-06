package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/internal/clientfs"

	"wonderland-gonline/client/wlo/app"
)

// localServerINI is the list used when SERVER.INI is missing: one local
// server, as in the capture install.
var localServerINI = []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")

// runClient starts the decompile port. With snapshot set it renders one
// frame to a PNG instead of opening a window.
func runClient(assets, serverINI, snapshot string) error {
	return runClientWithSprites(assets, serverINI, snapshot, "")
}

func runClientWithSprites(assets, serverINI, snapshot, sprites string) error {
	return runClientWithRenderer(assets, serverINI, snapshot, sprites, render.Auto)
}

func runClientWithRenderer(assets, serverINI, snapshot, sprites, renderer string) error {
	if err := render.Validate(renderer); err != nil {
		return err
	}
	root := assets
	userRoot := ""
	if strings.EqualFold(filepath.Ext(assets), ".zip") {
		var closeBundle func() error
		var err error
		root, closeBundle, err = clientfs.Mount(assets)
		if err != nil {
			return err
		}
		defer closeBundle()
		userRoot = filepath.Join(filepath.Dir(assets), "profiles")
		if serverINI == "" {
			serverINI = filepath.Join(filepath.Dir(assets), "SERVER.INI")
		}
	}
	c, err := app.New(app.Options{Renderer: renderer, Root: root, UserRoot: userRoot, SpritesRoot: sprites, ServerINI: serverINI, FallbackINI: localServerINI})
	if err != nil {
		return err
	}
	defer c.Net.Shutdown()
	if snapshot == "" {
		return app.Run(c)
	}
	defer func() { c.Sprites.WaitNative(); c.Sprites.Close() }()
	c.Frame()
	f, err := os.Create(snapshot)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, c.Screen.RGBA())
}
