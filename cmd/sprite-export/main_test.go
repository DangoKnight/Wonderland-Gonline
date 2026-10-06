package main

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/clientassets"
)

func TestArchiveRoundTripPreservesPadding(t *testing.T) {
	// Literal native headers describe one empty sprite with its 1024-byte
	// palette. Nonzero trailing padding is deliberately outside that entry.
	header, err := hex.DecodeString("6a61000000000000000001000000" + "05612e6a6d7000000000000000000000000000000e0400002a000000")
	if err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{'j', 'm'}, make([]byte, 1036)...)
	raw := append(append(header, payload...), 0x39, 0x4a, 0x5b)
	archive, err := clientassets.OpenSpriteArchive(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "decoded.jma")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = verifyArchive(file, archive, nil, nil, nil, hashBytes(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteAt([]byte{0}, int64(len(raw)-1)); err != nil {
		t.Fatal(err)
	}
	if err = verifyArchive(file, archive, nil, nil, nil, hashBytes(raw)); err == nil {
		t.Fatal("modified trailing padding accepted")
	}
}

func TestExportRetainsCompleteBytesWithoutJMAOutput(t *testing.T) {
	header, _ := hex.DecodeString("6a61000000000000000001000000" + "05612e6a6d7000000000000000000000000000000e0400002a000000")
	raw := append(append(header, append([]byte{'j', 'm'}, make([]byte, 1036)...)...), 0x39, 0x4a, 0x5b)
	root := t.TempDir()
	source := filepath.Join(root, "source.jma")
	os.WriteFile(source, raw, 0600)
	output := filepath.Join(root, "output")
	info, _, err := exportArchive(source, output, root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Outputs) != 1 || filepath.Ext(info.Outputs[0].Path) != ".json" {
		t.Fatalf("unexpected outputs: %+v", info.Outputs)
	}
	doc, err := clientassets.OpenSpriteJSON(filepath.Join(output, "sprites.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if len(doc.Archive.Entries) != 1 {
		t.Fatal("missing original sprite")
	}
	if _, err = os.Stat(filepath.Join(output, "sprites.decoded.jma")); !os.IsNotExist(err) {
		t.Fatal("native output remains")
	}
}
