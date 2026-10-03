package clientassets

import (
	"bytes"
	"crypto/subtle"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	jxanHeaderBytes         = 16
	jxanRecordBytes         = 64
	jxanNameBytes           = 17
	jxanNonceOffset         = 17
	jxanTagOffset           = 33
	jxanOffsetOffset        = 49
	jxanSizeOffset          = 53
	jxanWholeSpriteFlag     = 1
	jxanVersion             = 2
	nativeKeyLiteralAddress = 0x1658d4
	nativeKeyLengthOffset   = 4
)

type JXANRecord struct {
	Name   string `json:"name"`
	Offset uint32 `json:"offset"`
	Size   uint32 `json:"bytes"`
	Nonce  []byte `json:"nonce"`
	Tag    []byte `json:"tag"`
	Raw    []byte `json:"raw"`
}
type JXAN struct {
	KeyID   uint32       `json:"key_id"`
	Flags   uint16       `json:"flags"`
	Records []JXANRecord `json:"records"`
	Raw     []byte       `json:"raw"`
}

func ParseJXAN(data []byte) (*JXAN, error) {
	if len(data) < jxanHeaderBytes || string(data[:4]) != "JXAN" || binary.LittleEndian.Uint16(data[4:]) != jxanVersion {
		return nil, fmt.Errorf("unsupported JXAN header")
	}
	flags := binary.LittleEndian.Uint16(data[6:])
	count := binary.LittleEndian.Uint32(data[12:])
	if flags&jxanWholeSpriteFlag == 0 || uint64(jxanHeaderBytes)+uint64(count)*jxanRecordBytes != uint64(len(data)) {
		return nil, fmt.Errorf("invalid JXAN flags or record count")
	}
	out := &JXAN{KeyID: binary.LittleEndian.Uint32(data[8:]), Flags: flags, Raw: bytes.Clone(data)}
	seen := map[string]bool{}
	for i := range int(count) {
		row := data[jxanHeaderBytes+i*jxanRecordBytes:][:jxanRecordBytes]
		if int(row[0]) >= jxanNameBytes {
			return nil, fmt.Errorf("JXAN name too long")
		}
		name := string(row[1 : 1+int(row[0])])
		if name == "" || seen[name] {
			return nil, fmt.Errorf("empty or duplicate JXAN name")
		}
		seen[name] = true
		out.Records = append(out.Records, JXANRecord{Name: name, Offset: binary.LittleEndian.Uint32(row[jxanOffsetOffset:]), Size: binary.LittleEndian.Uint32(row[jxanSizeOffset:]), Nonce: bytes.Clone(row[jxanNonceOffset : jxanNonceOffset+nativeCipherBlockBytes]), Tag: bytes.Clone(row[jxanTagOffset : jxanTagOffset+nativeCipherBlockBytes]), Raw: bytes.Clone(row)})
	}
	return out, nil
}

// NativeSpriteKey reads the client-owned static asset key, never exposing it in
// exports or logs. Key correctness is independently checked by nonce and tags.
func NativeSpriteKey(executable []byte) ([]byte, error) {
	file, err := pe.NewFile(bytes.NewReader(executable))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	header, ok := file.OptionalHeader.(*pe.OptionalHeader32)
	if !ok {
		return nil, fmt.Errorf("expected native 32-bit client")
	}
	rva := uint32(nativeKeyLiteralAddress) - header.ImageBase
	for _, section := range file.Sections {
		if rva < section.VirtualAddress || uint64(rva) >= uint64(section.VirtualAddress)+uint64(section.Size) {
			continue
		}
		data, err := section.Data()
		if err != nil {
			return nil, err
		}
		offset := int(rva - section.VirtualAddress)
		if offset < nativeKeyLengthOffset || offset+nativeCipherKeyBytes > len(data) || binary.LittleEndian.Uint32(data[offset-nativeKeyLengthOffset:]) != nativeCipherKeyBytes {
			return nil, fmt.Errorf("unrecognized native key literal")
		}
		return bytes.Clone(data[offset : offset+nativeCipherKeyBytes]), nil
	}
	return nil, fmt.Errorf("native sprite key location is unavailable in this executable")
}

// DecodeJXANSprite authenticates ciphertext before decrypting it. A translated
// client may already contain plaintext while retaining its old JXAN metadata;
// in that case re-encryption must reproduce the authenticated tag.
func DecodeJXANSprite(master []byte, archive string, index *JXAN, record JXANRecord, source []byte) ([]byte, string, error) {
	if index == nil || len(record.Nonce) != nativeCipherBlockBytes || len(record.Tag) != nativeCipherBlockBytes || len(master) != nativeCipherKeyBytes || uint64(len(source)) != uint64(record.Size) {
		return nil, "", fmt.Errorf("JXAN key or sprite length mismatch")
	}
	base := strings.TrimSuffix(filepath.Base(archive), filepath.Ext(archive))
	nonce := nativeHMAC(master, []byte("jma-nonce:"+base+"|"+record.Name))
	if subtle.ConstantTimeCompare(nonce[:nativeCipherBlockBytes], record.Nonce) != 1 {
		return nil, "", fmt.Errorf("JXAN nonce mismatch: %s", record.Name)
	}
	keySuffix := make([]byte, 4)
	binary.LittleEndian.PutUint32(keySuffix, index.KeyID)
	encryptionKey := nativeHMAC(master, append([]byte("jma-enc:"), keySuffix...))
	macKey := nativeHMAC(master, append([]byte("jma-mac:"), keySuffix...))
	authenticated := func(ciphertext []byte) bool {
		message := append([]byte("JXANJMP"), []byte(record.Name)...)
		numbers := make([]byte, 8)
		binary.LittleEndian.PutUint32(numbers, record.Offset)
		binary.LittleEndian.PutUint32(numbers[4:], record.Size)
		message = append(message, numbers...)
		message = append(message, ciphertext...)
		tag := nativeHMAC(macKey[:], message)
		return subtle.ConstantTimeCompare(tag[:nativeCipherBlockBytes], record.Tag) == 1
	}
	if authenticated(source) {
		decoded := nativeCTR(encryptionKey[:], record.Nonce, source)
		if _, err := DecodeSprite(decoded); err != nil {
			return nil, "", err
		}
		return decoded, "decrypted-and-authenticated", nil
	}
	if len(source) >= 2 && string(source[:2]) == jmaSpriteSignature {
		ciphertext := nativeCTR(encryptionKey[:], record.Nonce, source)
		if authenticated(ciphertext) {
			if _, err := DecodeSprite(source); err != nil {
				return nil, "", err
			}
			return bytes.Clone(source), "already-plaintext-authenticated", nil
		}
	}
	return nil, "", fmt.Errorf("JXAN authentication failed: %s", record.Name)
}

// ReencryptJXANSprite restores an authenticated original for lossless export
// verification. It never publishes or stores the client asset key.
func ReencryptJXANSprite(master []byte, index *JXAN, record JXANRecord, decoded []byte) ([]byte, error) {
	if len(master) != nativeCipherKeyBytes || index == nil || len(record.Nonce) != nativeCipherBlockBytes || uint64(len(decoded)) != uint64(record.Size) {
		return nil, fmt.Errorf("invalid JXAN re-encryption inputs")
	}
	suffix := make([]byte, 4)
	binary.LittleEndian.PutUint32(suffix, index.KeyID)
	key := nativeHMAC(master, append([]byte("jma-enc:"), suffix...))
	return nativeCTR(key[:], record.Nonce, decoded), nil
}
