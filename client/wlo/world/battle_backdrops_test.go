package world

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientruntime"
)

func TestBattleBackdropSourceAndCompiledRecords(t *testing.T) {
	root := t.TempDir()
	a := login.NewAssets(root)
	raw := []byte(`{"entries":[{"name":"10017.map","decoded_hex":"1400000014000000000100010000000000006450015b0a0014002c019001"}]}`)
	if err := os.WriteFile(filepath.Join(root, groundExport), raw, 0600); err != nil {
		t.Fatal(err)
	}
	values, err := BattleBackdrops(a)
	if err != nil || values[10017].At(10, 20) != 91 || values[10017].At(0, 0) != 80 {
		t.Fatal("source backdrop decode", values, err)
	}
	compiled, err := clientruntime.Encode(map[uint16]clientassets.GroundBackdrops{10017: {Default: 50}})
	if err != nil {
		t.Fatal(err)
	}
	path := a.DataPath(clientruntime.BattleBackdropsFile)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, compiled, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, groundExport)); err != nil {
		t.Fatal(err)
	}
	values, err = BattleBackdrops(a)
	if err != nil || values[10017].At(10, 20) != 50 {
		t.Fatal("compiled record requires source data", values, err)
	}
}

func TestNativeClientBattleBackdropCatalog(t *testing.T) {
	needAssets(t)
	values, err := BattleBackdrops(assets)
	if err != nil || len(values) != 1147 || values[60001].At(2202, 1395) != 91 {
		t.Fatal("native backdrop catalog unavailable", len(values), err)
	}
}
