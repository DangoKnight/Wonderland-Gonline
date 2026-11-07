// Package importdata embeds the authored inputs needed by standalone extraction.
package importdata

import "embed"

// Resources preserves the same authored definitions as the checkout importer.
//
//go:embed server
var Resources embed.FS
