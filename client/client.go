package main

import (
	"image/png"
	"os"

	"wonderland-go/client/wlo/app"
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
	c, err := app.New(app.Options{Root: assets, SpritesRoot: sprites, ServerINI: serverINI, FallbackINI: localServerINI})
	if err != nil {
		return err
	}
	if snapshot == "" {
		return app.Run(c)
	}
	c.Frame()
	f, err := os.Create(snapshot)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, c.Screen.RGBA())
}
