package assetdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
)

// OpenReadOnly never creates a missing database and observes committed SQL edits.
func OpenReadOnly(path string) (*gorm.DB, error) { return openDatabaseMode(path, true, false) }

// ReadDocument combines document metadata with the indexed SQL collections.
// Indexed rows are authoritative, so SQL edits and deletions affect runtime
// loading instead of being hidden by the retained original document copy.
func ReadDocument(db *gorm.DB, asset string) ([]byte, error) {
	var document Document
	if err := db.Select("json").Where("asset = ?", asset).Take(&document).Error; err != nil {
		return nil, fmt.Errorf("asset %s: %w", asset, err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal([]byte(document.JSON), &value); err != nil {
		return nil, fmt.Errorf("asset %s document: %w", asset, err)
	}
	for _, collection := range indexedCollections {
		raw, exists := value[collection]
		if !exists || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
			continue
		}
		var records []Record
		if err := db.Select("json", "ordinal").Where("asset = ? AND collection = ?", asset, collection).Order("ordinal").Find(&records).Error; err != nil {
			return nil, err
		}
		rows := make([]json.RawMessage, len(records))
		for i, record := range records {
			if !json.Valid([]byte(record.JSON)) {
				return nil, fmt.Errorf("asset %s collection %s ordinal %d: invalid JSON", asset, collection, record.Ordinal)
			}
			rows[i] = json.RawMessage(record.JSON)
		}
		encoded, err := json.Marshal(rows)
		if err != nil {
			return nil, err
		}
		value[collection] = encoded
	}
	return json.Marshal(value)
}
