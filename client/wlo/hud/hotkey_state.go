package hud

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// Binding stores only a shortcut, never ownership or learned-skill state.
// Target/Pet identify an owned pet for Go's pet selector; native AC40 omits them.
type Binding struct {
	Kind   byte   `json:"kind"`
	ID     uint16 `json:"id"`
	Target byte   `json:"target,omitempty"`
	Pet    uint16 `json:"pet,omitempty"`
}
type Bindings [protocol.HotbarPages + 1][protocol.HotbarSlotsPerPage + 1]Binding

func (b Binding) Valid() bool {
	return b.Kind == 0 && b.ID == 0 && b.Target == 0 && b.Pet == 0 ||
		(b.Kind == protocol.HotbarSkill || b.Kind == protocol.HotbarItem) && b.ID != 0 && b.Target <= game.MaxPets && (b.Target == 0) == (b.Pet == 0)
}
func LoadBindings(path string) (Bindings, error) {
	var b Bindings
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	if err = json.Unmarshal(raw, &b); err != nil {
		return Bindings{}, err
	}
	for page, rows := range b {
		for slot, v := range rows {
			if !v.Valid() || (page == 0 || slot == 0) && v != (Binding{}) {
				return Bindings{}, fmt.Errorf("invalid hotbar binding %d/%d", page, slot)
			}
		}
	}
	return b, nil
}
func (b Bindings) Save(path string) error {
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hotbar-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
