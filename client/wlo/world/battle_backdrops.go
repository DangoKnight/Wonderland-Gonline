package world

import (
	"encoding/json"
	"os"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientruntime"
)

func BattleBackdrops(a login.Assets) (map[uint16]clientassets.GroundBackdrops, error) {
	var values map[uint16]clientassets.GroundBackdrops
	err := clientruntime.Read(a.DataPath(clientruntime.BattleBackdropsFile), &values)
	if err == nil {
		return values, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	raw, err := clientfs.ReadFile(a.DataPath(groundExport))
	if err != nil {
		return nil, err
	}
	var doc clientruntime.BackdropExport
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Decode()
}
