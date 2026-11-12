package main

import (
	"context"
	"path/filepath"
	"strings"

	"wonderland-gonline/client/wlo/app"
	"wonderland-gonline/client/wlo/headless"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/internal/clientfs"
)

func runNativeHeadless(ctx context.Context, assets, serverINI, sprites, language, address, token string) error {
	if err := headless.Validate(address, token); err != nil {
		return err
	}
	root, userRoot := assets, ""
	if strings.EqualFold(filepath.Ext(assets), ".zip") {
		mounted, closeBundle, err := clientfs.Mount(assets)
		if err != nil {
			return err
		}
		defer closeBundle()
		root = mounted
		userRoot = filepath.Join(filepath.Dir(assets), "profiles-headless")
		if serverINI == "" {
			serverINI = filepath.Join(filepath.Dir(assets), "SERVER.INI")
		}
	}
	c, err := app.New(app.Options{Root: root, UserRoot: userRoot, ServerINI: serverINI, FallbackINI: localServerINI, SpritesRoot: sprites, Language: language, Renderer: render.CPU})
	if err != nil {
		return err
	}
	return headless.Run(ctx, address, token, c)
}
