package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
)

//go:embed tent_tool_hosts_defaults.json
var tentToolHostsDefaults []byte

func defaultTentToolHosts() ([]uint16, error) {
	var values []uint16
	if err := json.Unmarshal(tentToolHostsDefaults, &values); err != nil {
		return values, err
	}
	return values, validateTentToolHosts(values)
}

func validateTentToolHosts(values []uint16) error {
	for _, id := range values {
		if id == 0 {
			return fmt.Errorf("invalid tent tool host")
		}
	}
	return nil
}

func migrateTentToolHosts(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_tent_tool_hosts", "tent_tool_hosts", func() error {
		values, err := defaultTentToolHosts()
		if err != nil {
			return err
		}
		return writeTentToolHosts(tx, values)
	})
}
