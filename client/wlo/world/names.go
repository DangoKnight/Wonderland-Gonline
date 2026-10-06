package world

import (
	"encoding/json"
	"fmt"
	"os"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientruntime"

	"wonderland-gonline/client/wlo/login"
)

// Extracted SceneData.dat (TFSceneData, FUN_0047be24 / FUN_0047c808) and
// eve.Emg.
const (
	sceneExport = "scene_data.json"
	eveExport   = "eve_data.json"
	talkExport  = "talk_data.json"
)

// Talks maps talk IDs to their text (Talk.dat, looked up by FUN_00332eac;
// the text is the record's +6).
func Talks(a login.Assets) (map[uint16]string, error) {
	var doc struct {
		Records []struct {
			Text struct {
				Text string `json:"text"`
			} `json:"text"`
			Fields struct {
				ID uint16 `json:"id"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(a.DataPath(talkExport), &doc); err != nil {
		return nil, err
	}
	out := make(map[uint16]string, len(doc.Records))
	for _, r := range doc.Records {
		out[r.Fields.ID] = r.Text.Text
	}
	return out, nil
}

// SceneNames maps scene IDs to their names.
func SceneNames(a login.Assets) (map[uint16]string, error) {
	var doc struct {
		Records []struct {
			Name struct {
				Text string `json:"text"`
			} `json:"name"`
			Fields struct {
				ID uint16 `json:"id"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(a.DataPath(sceneExport), &doc); err != nil {
		return nil, err
	}
	out := map[uint16]string{}
	for _, r := range doc.Records {
		out[r.Fields.ID] = r.Name.Text
	}
	return out, nil
}

// SceneMusic maps scene IDs to their background track (the record's
// second name at +0x1f, "BGM0007" for Ship Deck), played as
// Sound\<track>.wav (0x3bce95).
func SceneMusic(a login.Assets) (map[uint16]string, error) {
	var doc struct {
		Records []struct {
			Track struct {
				Text string `json:"text"`
			} `json:"secondary_name"`
			Fields struct {
				ID uint16 `json:"id"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(a.DataPath(sceneExport), &doc); err != nil {
		return nil, err
	}
	out := map[uint16]string{}
	for _, r := range doc.Records {
		if r.Track.Text != "" {
			out[r.Fields.ID] = r.Track.Text
		}
	}
	return out, nil
}

// SceneWeather maps scene IDs to their weather byte (the record's +0x29,
// tested by FUN_00334120; weather.SceneKind names the values). The export
// still calls it by its file offset.
func SceneWeather(a login.Assets) (map[uint16]byte, error) {
	var doc struct {
		Records []struct {
			Fields struct {
				ID      uint16 `json:"id"`
				Weather byte   `json:"unknown_u8_offset_35"`
			} `json:"fields"`
		} `json:"records"`
	}
	if err := readJSON(a.DataPath(sceneExport), &doc); err != nil {
		return nil, err
	}
	out := map[uint16]byte{}
	for _, r := range doc.Records {
		if r.Fields.Weather != 0 {
			out[r.Fields.ID] = r.Fields.Weather
		}
	}
	return out, nil
}

// MapScenes maps each map ID to its scene.
func MapScenes(a login.Assets) (map[uint16]uint16, error) {
	var compiled map[uint16]uint16
	if err := clientruntime.Read(a.DataPath(clientruntime.EventIndex), &compiled); err == nil {
		return compiled, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	var doc struct {
		Maps []struct {
			ID    uint16 `json:"id"`
			Scene uint16 `json:"scene"`
		} `json:"maps"`
	}
	if err := readJSON(a.DataPath(eveExport), &doc); err != nil {
		return nil, err
	}
	out := map[uint16]uint16{}
	for _, m := range doc.Maps {
		out[m.ID] = m.Scene
	}
	return out, nil
}

func readJSON(path string, v any) error {
	raw, err := clientfs.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
