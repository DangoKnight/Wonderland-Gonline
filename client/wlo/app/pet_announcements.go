package app

import "wonderland-gonline/internal/localization"

import (
	"bytes"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/game"
)

const petRecruitmentSound = `sound\wav0152.wav` // FUN_00423958

// Announce additions by identity, so roster refreshes and slot moves are quiet.
// Initial synchronization occurs before mapReady and must not announce pets.
func (c *Client) announceNewPets(before [game.MaxPets]inventory.UsePet) {
	if !c.mapReady || c.Services != nil && c.Services.Visible && c.Services.Mode == inventory.ServiceHotel {
		return
	}
	for _, pet := range c.InventoryState.Pets {
		if pet.ID == 0 {
			continue
		}
		found := false
		for _, old := range before {
			if old.ID == pet.ID {
				found = true
				break
			}
		}
		if found {
			continue
		}
		text := bytes.Clone(pet.Name)
		text = append(text, []byte(localization.UI("ui.app.joins_the_party_20b29322"))...)
		if c.battle.state.Active {
			c.Notices.Show(text, battleResultsDuration, c.Now())
		} else {
			c.petAnnouncements = append(c.petAnnouncements, text)
		}
		if c.Env.Sound != nil {
			c.Env.Sound(petRecruitmentSound)
		}
	}
}
