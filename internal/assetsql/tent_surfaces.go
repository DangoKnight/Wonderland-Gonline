package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed tent_surfaces_defaults.json
var tentSurfacesDefaults []byte

func defaultTentSurfaces() (map[uint16]byte, error) {
	var values map[uint16]byte
	if err := json.Unmarshal(tentSurfacesDefaults, &values); err != nil {
		return values, err
	}
	return values, validateTentSurfaces(values)
}

func validateTentSurfaces(values map[uint16]byte) error {
	for id, kind := range values {
		if id == 0 || kind != assets.TentSurfaceWallpaper && kind != assets.TentSurfaceFloor {
			return fmt.Errorf("invalid tent surface %d", id)
		}
	}
	return nil
}

func migrateTentSurfaces(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_tent_surfaces", "tent_surfaces", func() error {
		values, err := defaultTentSurfaces()
		if err != nil {
			return err
		}
		return writeTentSurfaces(tx, values)
	})
}
