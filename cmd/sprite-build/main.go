// Command sprite-build converts client sprites into sprite packs: one
// directory per archive with pack.json and RGBA PNG atlas pages
// (internal/spritepack). The client reads the editable atlases in
// data/sprites directly; built packs are optional and loaded through its
// -sprites flag.
//
// Sources are the editable export (data/sprites/<archive>/editable.json)
// or original jma/Jxa archives:
//
//	go run ./cmd/sprite-build -editable data/sprites -output var/client-spritepacks
//	go run ./cmd/sprite-build -jma ../Wonderland-Client/jma -output var/client-spritepacks
package main

import (
	"flag"
	"log"
	"path/filepath"
	"runtime"
	"strings"

	"wonderland-gonline/internal/spritepack"
)

func main() {
	editable := flag.String("editable", "", "editable export directory (data/sprites)")
	jma := flag.String("jma", "", "original jma directory")
	output := flag.String("output", filepath.Join("var", "client-spritepacks"), "sprite pack output directory")
	only := flag.String("archives", "", "comma-separated archive names to build (default all)")
	workers := flag.Int("workers", max(1, runtime.NumCPU()/2), "archives built in parallel")
	optimize := flag.Bool("optimize-editable", false, "optimize PNG sheets in place under -editable instead of building client packs")
	flag.Parse()
	if *optimize {
		if *editable == "" || *jma != "" {
			log.Fatal("-optimize-editable requires -editable and excludes -jma")
		}
		if err := spritepack.OptimizeEditableAll(*editable, strings.Split(*only, ","), log.Printf); err != nil {
			log.Fatal(err)
		}
		return
	}
	_, err := spritepack.BuildAll(spritepack.BuildOptions{
		Editable: *editable, JMA: *jma, Output: *output,
		Archives: strings.Split(*only, ","), Workers: *workers, Log: log.Printf,
	})
	if err != nil {
		log.Fatal(err)
	}
}
