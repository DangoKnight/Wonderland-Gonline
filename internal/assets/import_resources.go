package assets

import "embed"

// ImportResources supplies the authored Lucky Draw policy to offline extraction.
//
//go:embed lucky_draw_rules.json
var ImportResources embed.FS
