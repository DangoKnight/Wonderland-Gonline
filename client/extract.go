package main

import (
	"fmt"
	"os"

	"wonderland-gonline/client/internal/pictureexport"
	"wonderland-gonline/internal/itemexport"
	"wonderland-gonline/internal/skilleffectsexport"
	"wonderland-gonline/internal/spriteexport"
)

// runAssetTool is the extraction subprocess entry point. Calling this before
// game flag registration keeps the offline exporters independent of graphics.
func runAssetTool() bool {
	if len(os.Args) < 2 || os.Args[1] != "-asset-tool" {
		return false
	}
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "missing asset tool")
		os.Exit(2)
	}
	tool := os.Args[2]
	os.Args = append([]string{os.Args[0]}, os.Args[3:]...)
	switch tool {
	case "item-export":
		itemexport.Main()
	case "sprite-export":
		spriteexport.Main()
	case "pic-export":
		pictureexport.Main()
	case "skill-effects-export":
		skilleffectsexport.Main()
	default:
		fmt.Fprintln(os.Stderr, "unknown asset tool:", tool)
		os.Exit(2)
	}
	return true
}
