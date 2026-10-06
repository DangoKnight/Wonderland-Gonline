// Command item-export writes a complete decoded native Item.dat JSON asset.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"wonderland-gonline/internal/assets"
)

func main() {
	input := flag.String("input", "../Wonderland-Client/data/Item.dat", "native version-nine Item.dat")
	output := flag.String("output", "data/item_data.json", "decrypted JSON output")
	flag.Parse()
	if err := run(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input, output string) error {
	source, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	destination, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if source == destination {
		return fmt.Errorf("output must differ from input")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	export, err := assets.ExportNativeItems(data, filepath.ToSlash(source))
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	// Write completely before replacing the previous export.
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".items-export-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(encoded); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Chmod(0644); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporary.Name(), destination); err != nil {
		return err
	}
	fmt.Printf("Exported %d items to %s (source SHA-256 %s)\n", len(export.Items), destination, export.SourceSHA256)
	return nil
}
