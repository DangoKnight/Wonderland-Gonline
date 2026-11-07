package world

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/clientruntime"
)

func TestWarmTerrainUsesCompiledRecordsWithoutLooseExport(t *testing.T) {
	a := login.NewAssets(t.TempDir())
	if err := os.MkdirAll(a.DataPath(clientruntime.GroundDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := WarmTerrain(a); err != nil {
		t.Fatal("compiled assets incorrectly require loose ground JSON", err)
	}
	if _, err := os.Stat(filepath.Join(a.Data, groundExport)); !os.IsNotExist(err) {
		t.Fatal("fixture unexpectedly has loose terrain")
	}
}
