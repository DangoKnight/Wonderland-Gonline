// client-bundle is the offline compiler for editable data/ client assets.
package main

import (
	"flag"
	"fmt"
	"log"
	"wonderland-gonline/internal/clientbundle"
)

func main() {
	source := flag.String("source", "data", "editable asset directory")
	output := flag.String("output", "bin/client-assets.zip", "compiled bundle")
	contract := flag.String("contract", "client/asset-contract.json", "image dimension contract (initialized on first build)")
	optimized := flag.Bool("optimized", true, "compile runtime records and separate core/world/sprite/audio packs")
	flag.Parse()
	build := clientbundle.Build
	if *optimized {
		build = clientbundle.BuildRuntime
	}
	r, err := build(clientbundle.Options{Source: *source, Output: *output, Contract: *contract})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Client assets: %d files, %d reused, %.1f MiB source\n", r.Files, r.Reused, float64(r.SourceBytes)/(1<<20))
}
