package app

import (
	"bytes"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/game"
)

const petRecruitmentSound = `sound\wav0152.wav` // FUN_00423958

// Announce additions by identity, so roster refreshes and slot moves are quiet.
// Initial synchronization occurs before mapReady and must not announce pets.
func (c *Client) announceNewPets(before [game.MaxPets]inventory.UsePet) {
	if !c.mapReady {
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
		text = append(text, []byte(" joins the party")...)
		c.petAnnouncements = append(c.petAnnouncements, text)
		if c.Env.Sound != nil {
			c.Env.Sound(petRecruitmentSound)
		}
	}
}
