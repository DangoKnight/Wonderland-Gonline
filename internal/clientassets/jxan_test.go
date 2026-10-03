package clientassets

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// These literals were calculated independently in Python from the disassembled
// routines, using an algebraic AES S-box. They preserve native compatibility
// quirks and cover SHA padding boundaries and a full CTR counter wrap.
func TestNativeSpriteCryptoGolden(t *testing.T) {
	for _, tc := range []struct {
		size int
		want string
	}{
		{0, "864d4f4db7400d566f1e07be2d934bdfda3c77f5081b6a2a489ef78605859086"},
		{3, "19b270f6b6eea584cb64e7837096a09e9b6011b2d27c520c013bd561550ba55c"},
		{55, "f4540fce250432f6befcfa36d8c2db18c03b2c22bb37208fff7df810140a1300"},
		{56, "b4bb0e99336052ceea28b39d5bc814043dd1501b5223be0226187cacd911417d"},
		{64, "c75766d8d9ac28902f5ea5948010849150e8b41d7987b502fca856779dbe67d7"},
		{129, "546fa4c829208cbfad8ae41a75ecc43344889e93d4f6b2e5af5f0c7ae795de82"},
	} {
		got := nativeSHA256(bytes.Repeat([]byte{'a'}, tc.size))
		if hex.EncodeToString(got[:]) != tc.want {
			t.Fatalf("hash size %d: %x", tc.size, got)
		}
	}
	key := make([]byte, 128)
	for i := range key {
		key[i] = byte(i)
	}
	got := nativeHMAC(key, []byte("jma-enc:\x01\x00\x00\x00"))
	if hex.EncodeToString(got[:]) != "7c6b8d66d3a9991634f22e435f5b4797b3312454580379c6d071553026517c8e" {
		t.Fatalf("HMAC: %x", got)
	}
	schedule := newNativeCipher(key[:32])
	var block [16]byte
	for i := range block {
		block[i] = byte(i)
	}
	encrypted := schedule.encrypt(block)
	if hex.EncodeToString(encrypted[:]) != "b9182af14510c6a735dea5a18d0af543" {
		t.Fatalf("cipher: %x", encrypted)
	}
	data := make([]byte, 37)
	for i := range data {
		data[i] = byte(i)
	}
	nonce := bytes.Repeat([]byte{255}, 16)
	ciphertext := nativeCTR(key[:32], nonce, data)
	if hex.EncodeToString(ciphertext) != "d5791c0dc19cb253c70123499f35a4e8667b8d983c099ed5d35b828d7c74dcfae394d0a9af" {
		t.Fatalf("CTR: %x", ciphertext)
	}
	if !bytes.Equal(nativeCTR(key[:32], nonce, ciphertext), data) {
		t.Fatal("CTR reversal")
	}
}

func TestTranslatedClientEncryptedSprites(t *testing.T) {
	root := os.Getenv("WONDERLAND_SPRITE_CLIENT")
	if root == "" {
		root = filepath.Join("..", "..", "..", "Wonderland-Client")
	}
	executable, err := os.ReadFile(filepath.Join(root, "aLogin.exe"))
	if os.IsNotExist(err) {
		t.Skip("translated client not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	key, err := NativeSpriteKey(executable)
	if err != nil {
		t.Fatal(err)
	}
	for _, stem := range []string{"002s2", "006_c02"} {
		raw, err := os.ReadFile(filepath.Join(root, "jma", stem+".jxan"))
		if err != nil {
			t.Fatal(err)
		}
		index, err := ParseJXAN(raw)
		if err != nil {
			t.Fatal(err)
		}
		archive, err := os.ReadFile(filepath.Join(root, "jma", stem+".jma"))
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range index.Records {
			source := archive[int(record.Offset):int(record.Offset+record.Size)]
			decoded, status, err := DecodeJXANSprite(key, stem+".jma", index, record, source)
			if err != nil || status != "decrypted-and-authenticated" {
				t.Fatalf("%s: %s %v", record.Name, status, err)
			}
			restored, err := ReencryptJXANSprite(key, index, record, decoded)
			if err != nil || !bytes.Equal(restored, source) {
				t.Fatalf("%s: restoration: %v", record.Name, err)
			}
			already, status, err := DecodeJXANSprite(key, stem+".jma", index, record, decoded)
			if err != nil || status != "already-plaintext-authenticated" || !bytes.Equal(already, decoded) {
				t.Fatalf("already decoded %s: %v", record.Name, err)
			}
			damaged := bytes.Clone(source)
			damaged[len(damaged)/2] ^= 1
			if _, _, err := DecodeJXANSprite(key, stem+".jma", index, record, damaged); err == nil {
				t.Fatal("corrupted ciphertext accepted")
			}
			bad := record
			bad.Tag = bytes.Clone(record.Tag)
			bad.Tag[0] ^= 1
			if _, _, err := DecodeJXANSprite(key, stem+".jma", index, bad, source); err == nil {
				t.Fatal("corrupted tag accepted")
			}
			if _, _, err := DecodeJXANSprite(key, "wrong.jma", index, record, source); err == nil {
				t.Fatal("wrong archive accepted")
			}
		}
		for _, invalid := range [][]byte{nil, raw[:len(raw)-1], append(bytes.Clone(raw), 0)} {
			if _, err := ParseJXAN(invalid); err == nil {
				t.Fatal("invalid JXAN accepted")
			}
		}
	}
}
