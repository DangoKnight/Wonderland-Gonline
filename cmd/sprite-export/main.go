// Command sprite-export saves lossless sprite JSON and editable PNG atlases.
package main

import (
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wonderland-gonline/internal/clientassets"
)

const (
	exportSchemaVersion     = 1
	spriteHeaderBytes       = 14
	spriteEntryBytes        = 28
	spriteFrameBytes        = 40
	spritePaletteBytes      = 1024
	actionRecordBytes       = 80
	spriteCountOffset       = 10
	framePixelBytesOffset   = 20
	framePixelOffsetOffset  = 24
	encryptionHeaderBytes   = 16
	encryptionFormatVersion = 2
)

type outputInfo struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type sourceInfo struct {
	Source    string       `json:"source"`
	Bytes     int64        `json:"bytes"`
	SHA256    string       `json:"sha256"`
	Format    string       `json:"format"`
	Outputs   []outputInfo `json:"outputs"`
	Sprites   int          `json:"sprites,omitempty"`
	Frames    int          `json:"frames,omitempty"`
	Encrypted int          `json:"encrypted_sprites,omitempty"`
}
type manifest struct {
	SchemaVersion           int          `json:"schema_version"`
	KeySource               string       `json:"key_source"`
	KeySourceSHA256         string       `json:"key_source_sha256"`
	Sources                 []sourceInfo `json:"sources"`
	Sprites                 int          `json:"sprites"`
	Frames                  int          `json:"frames"`
	Decrypted               int          `json:"decrypted_sprites"`
	AlreadyPlaintext        int          `json:"already_plaintext_authenticated"`
	MissingCompanionRecords int          `json:"missing_companion_records"`
}
type frame struct {
	Index        int    `json:"index"`
	Name         string `json:"name"`
	CanvasWidth  int    `json:"canvas_width"`
	CanvasHeight int    `json:"canvas_height"`
	X            int    `json:"anchor_x"`
	Y            int    `json:"anchor_y"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Stride       int    `json:"stride"`
	PixelOffset  uint32 `json:"pixel_offset_in_sprite"`
	PixelBytes   uint32 `json:"pixel_bytes"`
	Unknown36    uint32 `json:"unknown_u32_offset_36"`
	RecordHex    string `json:"record_hex"`
}
type sprite struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	Offset         uint32  `json:"file_offset"`
	Bytes          uint32  `json:"bytes"`
	Status         string  `json:"status"`
	SHA256         string  `json:"decoded_sha256"`
	HeaderHex      string  `json:"header_hex"`
	PaletteBGRAHex string  `json:"palette_bgra_hex"`
	Frames         []frame `json:"frames"`
}
type archiveExport struct {
	SchemaVersion  int      `json:"schema_version"`
	Source         string   `json:"source"`
	SourceSHA256   string   `json:"source_sha256"`
	DecodedArchive string   `json:"decoded_archive_zlib_base64"`
	DecodedBytes   int64    `json:"decoded_archive_bytes"`
	PayloadSHA256  string   `json:"payload_sha256"`
	HeaderHex      string   `json:"header_hex"`
	Sprites        []sprite `json:"sprites"`
}

func hashBytes(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func jsonWrite(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".sprite-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(value)
	if err == nil {
		err = file.Chmod(0644)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
func outputMetadata(root, path string) (outputInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return outputInfo{}, err
	}
	digest, err := hashFile(path)
	if err != nil {
		return outputInfo{}, err
	}
	relative, err := filepath.Rel(root, path)
	return outputInfo{Path: filepath.ToSlash(relative), Bytes: info.Size(), SHA256: digest}, err
}

func run(input, executable, output string) error {
	input, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	input, err = filepath.EvalSymlinks(input)
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(output, 0755); err != nil {
		return err
	}
	output, err = filepath.EvalSymlinks(output)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(input, output)
	if err != nil {
		return err
	}
	if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("output must be outside source directory")
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	executableBytes, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	key, err := clientassets.NativeSpriteKey(executableBytes)
	if err != nil {
		return err
	}
	result := manifest{SchemaVersion: exportSchemaVersion, KeySource: executable, KeySourceSHA256: hashBytes(executableBytes)}
	files, err := os.ReadDir(input)
	if err != nil {
		return err
	}
	names := map[string]string{}
	indexes := map[string]*clientassets.JXAN{}
	statuses := map[string]map[string]string{}
	spriteCounts := map[string]int{}
	for _, file := range files {
		if file.IsDir() {
			return fmt.Errorf("unexpected sprite subdirectory: %s", file.Name())
		}
		extension := strings.ToLower(filepath.Ext(file.Name()))
		if extension != ".jma" && extension != ".jxa" && extension != ".jxan" {
			return fmt.Errorf("unsupported sprite file: %s", file.Name())
		}
		lower := strings.ToLower(file.Name())
		if names[lower] != "" {
			return fmt.Errorf("case-insensitive filename collision")
		}
		names[lower] = file.Name()
		if extension == ".jxan" {
			data, err := os.ReadFile(filepath.Join(input, file.Name()))
			if err != nil {
				return err
			}
			index, err := clientassets.ParseJXAN(data)
			if err != nil {
				return err
			}
			stem := strings.ToLower(strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
			indexes[stem] = index
		}
	}
	// Decode archives first; JXAN records refer to their corresponding JMA bytes.
	for _, file := range files {
		if strings.ToLower(filepath.Ext(file.Name())) != ".jma" {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		if names[stem+".jxa"] == "" {
			return fmt.Errorf("missing JXA companion: %s", file.Name())
		}
		info, plain, err := exportArchive(filepath.Join(input, file.Name()), filepath.Join(output, stem), output, key, indexes[stem])
		if err != nil {
			return fmt.Errorf("%s: %w", file.Name(), err)
		}
		statuses[stem] = plain
		spriteCounts[stem] = info.Sprites
		result.Sources = append(result.Sources, info)
		result.Sprites += info.Sprites
		result.Frames += info.Frames
		result.Decrypted += info.Encrypted
		for _, status := range plain {
			if status == "already-plaintext-authenticated" {
				result.AlreadyPlaintext++
			}
		}
		fmt.Printf("%s: %d sprites, %d frames, %d decrypted\n", file.Name(), info.Sprites, info.Frames, info.Encrypted)
	}
	for _, file := range files {
		extension := strings.ToLower(filepath.Ext(file.Name()))
		if extension == ".jma" {
			continue
		}
		path := filepath.Join(input, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		stem := strings.ToLower(strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())))
		if names[stem+".jma"] == "" && extension == ".jxa" {
			return fmt.Errorf("metadata has no archive: %s", file.Name())
		}
		destination := filepath.Join(output, stem, "actions.json")
		format := "plaintext-jxa"
		var value any
		if extension == ".jxa" {
			if len(data) < spriteHeaderBytes || string(data[:2]) != "jx" {
				return fmt.Errorf("invalid JXA signature")
			}
			count := binary.LittleEndian.Uint32(data[spriteCountOffset:])
			if int(count) != spriteCounts[stem] || uint64(spriteHeaderBytes)+uint64(count)*actionRecordBytes != uint64(len(data)) {
				return fmt.Errorf("invalid JXA record count")
			}
			rows := make([]any, 0, count)
			for i := range int(count) {
				row := data[spriteHeaderBytes+i*actionRecordBytes:][:actionRecordBytes]
				counts := make([]int, len(row))
				for j, v := range row {
					counts[j] = int(v)
				}
				rows = append(rows, map[string]any{"sprite_index": i, "action_frame_counts": counts, "record_hex": hex.EncodeToString(row)})
			}
			value = map[string]any{"schema_version": exportSchemaVersion, "source": path, "source_sha256": hashBytes(data), "header_hex": hex.EncodeToString(data[:spriteHeaderBytes]), "records": rows}
		} else {
			destination = filepath.Join(output, stem, "encryption_metadata.json")
			format = "plaintext-jxan-metadata"
			index := indexes[stem]
			rows := make([]any, 0, len(index.Records))
			for _, row := range index.Records {
				status, exists := statuses[stem][row.Name]
				if !exists {
					if names[stem+".jma"] != "" {
						return fmt.Errorf("unprocessed JXAN record: %s", row.Name)
					}
					status = "missing-companion-archive"
					result.MissingCompanionRecords++
				}
				rows = append(rows, map[string]any{"name": row.Name, "file_offset": row.Offset, "bytes": row.Size, "nonce_hex": hex.EncodeToString(row.Nonce), "authentication_tag_hex": hex.EncodeToString(row.Tag), "record_hex": hex.EncodeToString(row.Raw), "source_status": status})
			}
			value = map[string]any{"schema_version": exportSchemaVersion, "source": path, "source_sha256": hashBytes(data), "version": encryptionFormatVersion, "flags": index.Flags, "key_id": index.KeyID, "header_hex": hex.EncodeToString(data[:encryptionHeaderBytes]), "records": rows}
		}
		if err = jsonWrite(destination, value); err != nil {
			return err
		}
		meta, err := outputMetadata(output, destination)
		if err != nil {
			return err
		}
		result.Sources = append(result.Sources, sourceInfo{Source: path, Bytes: int64(len(data)), SHA256: hashBytes(data), Format: format, Outputs: []outputInfo{meta}})
	}
	if err = jsonWrite(filepath.Join(output, "sprite_manifest.json"), result); err != nil {
		return err
	}
	fmt.Printf("Exported all %d files: %d sprites, %d frames, %d decrypted, %d already plaintext authenticated\n", len(result.Sources), result.Sprites, result.Frames, result.Decrypted, result.AlreadyPlaintext)
	return nil
}

func exportArchive(source, directory, root string, key []byte, index *clientassets.JXAN) (sourceInfo, map[string]string, error) {
	var info sourceInfo
	statuses := map[string]string{}
	sourceFile, err := os.Open(source)
	if err != nil {
		return info, statuses, err
	}
	defer sourceFile.Close()
	stat, err := sourceFile.Stat()
	if err != nil {
		return info, statuses, err
	}
	sourceHash, err := hashFile(source)
	if err != nil {
		return info, statuses, err
	}
	archive, err := clientassets.OpenSpriteArchive(sourceFile, stat.Size())
	if err != nil {
		return info, statuses, err
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		return info, statuses, err
	}
	temporary, err := os.CreateTemp(directory, ".decoded-*")
	if err != nil {
		return info, statuses, err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if _, err = io.Copy(temporary, sourceFile); err != nil {
		return info, statuses, err
	}
	header := make([]byte, spriteHeaderBytes)
	if _, err = sourceFile.ReadAt(header, 0); err != nil {
		return info, statuses, err
	}
	document := archiveExport{SchemaVersion: exportSchemaVersion, Source: source, SourceSHA256: sourceHash, DecodedBytes: stat.Size(), HeaderHex: hex.EncodeToString(header)}
	encryption := map[string]clientassets.JXANRecord{}
	if index != nil {
		for _, record := range index.Records {
			encryption[record.Name] = record
		}
	}
	info = sourceInfo{Source: source, Bytes: stat.Size(), SHA256: sourceHash, Format: "decompiled-sprites", Sprites: len(archive.Entries)}
	seen := map[string]bool{}
	for entryIndex, entry := range archive.Entries {
		if seen[entry.Name] {
			return info, statuses, fmt.Errorf("duplicate sprite name: %s", entry.Name)
		}
		seen[entry.Name] = true
		data := make([]byte, entry.Size)
		if _, err = sourceFile.ReadAt(data, int64(entry.Offset)); err != nil {
			return info, statuses, err
		}
		status := "plaintext"
		if record, exists := encryption[entry.Name]; exists {
			if entry.Offset != record.Offset || entry.Size != record.Size {
				return info, statuses, fmt.Errorf("JXAN archive bounds mismatch: %s", entry.Name)
			}
			data, status, err = clientassets.DecodeJXANSprite(key, filepath.Base(source), index, record, data)
			if err != nil {
				return info, statuses, err
			}
			statuses[entry.Name] = status
			if status == "decrypted-and-authenticated" {
				info.Encrypted++
			}
			if _, err = temporary.WriteAt(data, int64(entry.Offset)); err != nil {
				return info, statuses, err
			}
		}
		decoded, err := clientassets.DecodeSprite(data)
		if err != nil {
			return info, statuses, fmt.Errorf("sprite %s: %w", entry.Name, err)
		}
		paletteOffset := spriteHeaderBytes + len(decoded.Frames)*spriteFrameBytes
		row := sprite{Index: entryIndex, Name: entry.Name, Offset: entry.Offset, Bytes: entry.Size, Status: status, SHA256: hashBytes(data), HeaderHex: hex.EncodeToString(data[:spriteHeaderBytes]), PaletteBGRAHex: hex.EncodeToString(data[paletteOffset : paletteOffset+spritePaletteBytes])}
		for frameIndex, f := range decoded.Frames {
			native := data[spriteHeaderBytes+frameIndex*spriteFrameBytes:][:spriteFrameBytes]
			row.Frames = append(row.Frames, frame{Index: frameIndex, Name: f.Name, CanvasWidth: f.CanvasWidth, CanvasHeight: f.CanvasHeight, X: f.X, Y: f.Y, Width: f.Width, Height: f.Height, Stride: f.Stride, PixelOffset: binary.LittleEndian.Uint32(native[framePixelOffsetOffset:]), PixelBytes: binary.LittleEndian.Uint32(native[framePixelBytesOffset:]), Unknown36: f.Extra, RecordHex: hex.EncodeToString(native)})
		}
		info.Frames += len(row.Frames)
		document.Sprites = append(document.Sprites, row)
	}
	if len(statuses) != len(encryption) {
		return info, statuses, fmt.Errorf("JXAN references sprites absent from archive")
	}
	if err = verifyArchive(temporary, archive, key, index, statuses, sourceHash); err != nil {
		return info, statuses, err
	}
	if err = temporary.Chmod(0644); err != nil {
		return info, statuses, err
	}
	if err = temporary.Close(); err != nil {
		return info, statuses, err
	}
	document.PayloadSHA256, err = hashFile(temporary.Name())
	if err != nil {
		return info, statuses, err
	}
	// Validate complete source preservation: changed regions are exactly the
	// authenticated encrypted records; headers, padding and gaps stay unchanged.
	if current, err := hashFile(source); err != nil || current != sourceHash {
		return info, statuses, fmt.Errorf("source changed during export")
	}
	compressedSource, err := os.Open(temporary.Name())
	if err != nil {
		return info, statuses, err
	}
	var encoded strings.Builder
	base := base64.NewEncoder(base64.StdEncoding, &encoded)
	compressed := zlib.NewWriter(base)
	_, copyErr := io.Copy(compressed, compressedSource)
	compressedSource.Close()
	compressedErr := compressed.Close()
	baseErr := base.Close()
	if copyErr != nil {
		return info, statuses, copyErr
	}
	if compressedErr != nil {
		return info, statuses, compressedErr
	}
	if baseErr != nil {
		return info, statuses, baseErr
	}
	document.DecodedArchive = encoded.String()
	metadataPath := filepath.Join(directory, "sprites.json")
	if err = jsonWrite(metadataPath, document); err != nil {
		return info, statuses, err
	}
	for _, path := range []string{metadataPath} {
		output, err := outputMetadata(root, path)
		if err != nil {
			return info, statuses, err
		}
		info.Outputs = append(info.Outputs, output)
	}
	return info, statuses, nil
}

func preview(archivePath, name string, frameIndex int, output string) error {
	var archive *clientassets.SpriteArchive
	if strings.EqualFold(filepath.Ext(archivePath), ".json") {
		source, err := clientassets.OpenSpriteJSON(archivePath)
		if err != nil {
			return err
		}
		defer source.Close()
		archive = source.Archive
	} else {
		file, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			return err
		}
		archive, err = clientassets.OpenSpriteArchive(file, stat.Size())
		if err != nil {
			return err
		}
	}

	sprite, err := archive.Sprite(archive.Find(name))
	if err != nil {
		return err
	}
	if frameIndex < 0 || frameIndex >= len(sprite.Frames) {
		return fmt.Errorf("frame index out of range")
	}
	f := sprite.Frames[frameIndex]
	if f.Width == 0 || f.Height == 0 {
		return fmt.Errorf("empty frame cannot be saved as PNG")
	}
	palette := make(color.Palette, len(sprite.Palette))
	for i, rgb := range sprite.Palette {
		alpha := byte(255)
		if i == 0 {
			alpha = 0
		}
		palette[i] = color.NRGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: alpha}
	}
	pixels := image.NewPaletted(image.Rect(0, 0, f.Width, f.Height), palette)
	for y := 0; y < f.Height; y++ {
		copy(pixels.Pix[y*pixels.Stride:y*pixels.Stride+f.Width], f.Pixels[y*f.Stride:y*f.Stride+f.Width])
	}
	if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	target, err := os.Create(output)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(target, pixels)
	closeErr := target.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func main() {
	input := flag.String("input", "../Wonderland-Client/jma", "native sprite resource directory")
	executable := flag.String("client-exe", "../Wonderland-Client/aLogin.exe", "matching native client supplying the sprite asset key")
	output := flag.String("output", "data/sprites", "decoded sprite directory")
	previewArchive := flag.String("preview-archive", "", "export a PNG frame from sprites.json or a native source archive")
	previewSprite := flag.String("sprite", "1001.jmp", "sprite name for PNG export")
	frameIndex := flag.Int("frame", 0, "frame index for PNG export")
	editable := flag.Bool("editable", false, "decrypt original sibling archives and export editable PNG sheets and JSON")
	selectedArchive := flag.String("archive", "", "limit editable export to one archive stem")
	overwriteEdits := flag.Bool("overwrite-edits", false, "replace existing editable sprites when regenerating")
	verifyEdits := flag.Bool("verify-editable", false, "validate PNG sheets and animation JSON under -output")
	compareOriginal := flag.Bool("compare-original", false, "compare editable images with lossless sprite JSON pixels and placement")
	flag.Parse()
	var err error
	if *verifyEdits {
		err = verifyEditable(*output, *selectedArchive, *compareOriginal)
	} else if *editable {
		err = exportEditableFromSource(*input, *executable, *output, *selectedArchive, *overwriteEdits)
	} else if *previewArchive != "" {
		err = preview(*previewArchive, *previewSprite, *frameIndex, *output)
	} else {
		err = run(*input, *executable, *output)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// verifyArchive reconstructs the original archive, including unparsed gaps and
// padding, and checks its entire SHA-256 digest before publishing the export.
func verifyArchive(file *os.File, archive *clientassets.SpriteArchive, key []byte, index *clientassets.JXAN, statuses map[string]string, expected string) error {
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	entries := append([]clientassets.SpriteEntry(nil), archive.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Offset < entries[j].Offset })
	records := map[string]clientassets.JXANRecord{}
	if index != nil {
		for _, record := range index.Records {
			records[record.Name] = record
		}
	}
	hash := sha256.New()
	cursor := int64(0)
	for _, entry := range entries {
		if statuses[entry.Name] != "decrypted-and-authenticated" {
			continue
		}
		offset := int64(entry.Offset)
		if offset < cursor {
			return fmt.Errorf("overlapping encrypted sprites")
		}
		if _, err = io.Copy(hash, io.NewSectionReader(file, cursor, offset-cursor)); err != nil {
			return err
		}
		data := make([]byte, entry.Size)
		if _, err = file.ReadAt(data, offset); err != nil {
			return err
		}
		original, err := clientassets.ReencryptJXANSprite(key, index, records[entry.Name], data)
		if err != nil {
			return err
		}
		hash.Write(original)
		cursor = offset + int64(entry.Size)
	}
	if _, err = io.Copy(hash, io.NewSectionReader(file, cursor, stat.Size()-cursor)); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("sprite archive round-trip checksum mismatch")
	}
	return nil
}
