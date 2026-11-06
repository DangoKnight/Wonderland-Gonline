package inventory

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"time"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	itemHPStatus            = 25
	itemSPStatus            = 26
	itemHPBonusStatus       = 207
	itemSPBonusStatus       = 208
	itemAmityStatus         = 64
	recoveryValueBase       = 100
	itemConsumableSlot      = 8
	potentialReplyTimeout   = 5 * time.Second
	recoveryUnavailableText = "ui.inventory.this_item_cannot_benefit_the_selected_ta_c5cf5353"
)

type potentialRequest struct {
	target byte
	before byte
	petID  uint16
	sent   time.Time
}

func potentialPill(id uint16) bool {
	return id == game.ItemPotentialPill || id == game.ItemGoldenPotentialPill || id == game.ItemSuperPotentialPill
}
func (f *Form) useBag(slot byte) {
	it := f.State.Bag[slot-1]
	if it.Locked {
		f.notice(localization.UI("ui.inventory.item_locked_can_t_use_62ebd9b8"))
		return
	}
	def := f.State.Items[it.ID].Definition
	if def.Type == game.VehicleType {
		f.send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlPlaceVehicleManual, slot}.U16(it.ID))
		return
	}
	if potentialPill(it.ID) {
		if f.potentialRequest != nil {
			if f.itemUseTime().Sub(f.potentialRequest.sent) < potentialReplyTimeout {
				f.notice(localization.UI("ui.inventory.waiting_for_the_previous_potential_pill__14385cf1"))
				return
			}
			f.potentialRequest = nil
		}
		f.openUse(slot, it)
		return
	}
	if it.ID == itemRemoteControl {
		f.openRemote(slot)
		return
	}
	if f.State.equipmentSlot(it.ID) != 0 {
		f.equipBag(slot)
		return
	}
	// Native Item.dat use category 8 is a consumable, even when its effects
	// are not HP/SP recovery. Preserve the selected target in its request.
	if def.EquipSlot == itemConsumableSlot {
		f.UseSelected(slot, 1, f.Selected, it)
		return
	}
	for i, status := range def.Status {
		if status == itemAmityStatus || ((status == itemHPStatus || status == itemHPBonusStatus || status == itemSPStatus || status == itemSPBonusStatus) && def.Values[i] > recoveryValueBase) {
			f.UseSelected(slot, 1, f.Selected, it)
			return
		}
	}
	// Tents, reward packs, vouchers, rods and special tools are dispatched by
	// the server's AC23:96 handler. Unknown items never gain a local effect.
	f.send([]byte{protocol.CommandInventory, protocol.InventoryUse, slot})
}

// UseSelected revalidates the dialog snapshot and owned target before sending.
// Bag counts, vitals and potential are changed exclusively by server replies.
func (f *Form) UseSelected(slot, count, target byte, expected game.Item) bool {
	if !f.allowed() || f.Send == nil || slot < 1 || slot > game.BagSize || count == 0 || target > game.MaxPets || f.State.Bag[slot-1] != expected || expected.Empty() || expected.Locked || count > expected.Count {
		return false
	}
	if target != 0 && f.State.Pets[target-1].ID == 0 {
		f.notice(localization.UI("ui.inventory.that_pet_is_no_longer_available_33a51275"))
		return false
	}
	if potentialPill(expected.ID) {
		if count != 1 || f.potentialRequest != nil {
			return false
		}
		before := f.Stats.Potential
		petID := uint16(0)
		if target != 0 {
			before = f.State.Pets[target-1].Potential
			petID = f.State.Pets[target-1].ID
		}
		if before >= game.PotentialMaximum {
			f.notice(localization.UI("ui.inventory.reached_max_potential_9f6c9353"))
			return false
		}
		if expected.ID == game.ItemGoldenPotentialPill && before < game.GoldenPotentialMinimum {
			f.notice(localization.UI("ui.inventory.golden_potential_pills_require_at_least__95646da8"))
			return false
		}
		now := f.itemUseTime()
		f.potentialRequest = &potentialRequest{target: target, before: before, petID: petID, sent: now}
		f.send([]byte{protocol.CommandInventory, protocol.InventoryPotentialPill, slot, target})
	} else {
		def := f.State.Items[expected.ID].Definition
		if target == 0 && def.Type == game.ItemTypePetStatFood {
			if f.Warning != nil {
				f.Warning(localization.UI("ui.inventory.select_a_pet_for_this_food_0199587d"))
			} else {
				f.notice(localization.UI("ui.inventory.select_a_pet_for_this_food_0199587d"))
			}
			return false
		}
		if target == 0 && (def.Status[0] == itemAmityStatus || def.Status[1] == itemAmityStatus) {
			f.notice(localization.UI("ui.inventory.select_a_pet_for_this_item_7008c86c"))
			return false
		}
		stats := f.Stats
		if target != 0 {
			stats = &f.State.Pets[target-1].Stats
		}
		var hpGain, spGain int32
		for i, status := range def.Status {
			switch status {
			case itemHPStatus, itemHPBonusStatus:
				hpGain += max(0, def.Values[i]-recoveryValueBase)
			case itemSPStatus, itemSPBonusStatus:
				spGain += max(0, def.Values[i]-recoveryValueBase)
			}
		}
		if hpGain > 0 || spGain > 0 {
			hpNeeded := hpGain > 0 && (stats.MaxHP == 0 || stats.HP < stats.MaxHP)
			spNeeded := spGain > 0 && (stats.MaxSP == 0 || stats.SP < stats.MaxSP)
			if !hpNeeded && !spNeeded {
				if f.Warning != nil {
					f.Warning(localization.UI(recoveryUnavailableText))
				} else {
					f.notice(localization.UI(recoveryUnavailableText))
				}
				return false
			}
		}
		f.send([]byte{protocol.CommandInventory, protocol.InventoryItemUse, slot, count, target, 0})
	}
	return true
}

func (f *Form) itemUseTime() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Form) ResetUse() { f.potentialRequest = nil }

// PotentialReply's processed flag means an attempt was committed, including a
// failed roll. It must never be interpreted as a success flag.
func (f *Form) PotentialReply(p []byte) bool {
	if len(p) != 5 || p[0] != protocol.CommandInventory || p[1] != protocol.InventoryPotentialPillResult || p[2] > protocol.PotentialPillProcessed || p[3] > game.MaxPets || p[4] > game.PotentialMaximum {
		return false
	}
	pending := f.potentialRequest
	if pending == nil || pending.target != p[3] {
		return false
	}
	f.potentialRequest = nil
	if p[2] == protocol.PotentialPillRejected {
		if f.Warning != nil {
			f.Warning(localization.UI("ui.inventory.the_potential_pill_could_not_be_used_e4a2268f"))
		}
		return true
	}
	if p[3] == 0 {
		f.Stats.Potential = p[4]
	} else if f.State.Pets[p[3]-1].ID == pending.petID {
		f.State.Pets[p[3]-1].Potential = p[4]
		f.State.Pets[p[3]-1].Stats.Potential = p[4]
	}
	switch {
	case p[4] > pending.before:
		if f.Env != nil && f.Env.Sound != nil {
			f.Env.Sound("sound\\SEB0300.wav")
		}
	case p[4] < pending.before:
	default:
		if f.UseDialog != nil && f.UseDialog.item.ID == game.ItemGoldenPotentialPill && f.Env != nil && f.Env.Sound != nil {
			f.Env.Sound("sound\\wav0152.wav")
		}
	}
	return true
}

const (
	petListPrefixBytes     = 30 // Fixed fields, including the name length.
	petNameLengthOffset    = 29
	petSkillRecordBytes    = 5
	petSkillBytes          = 3 * petSkillRecordBytes
	petEquipmentBytes      = EquipmentSlots * equipmentRecordBytes
	petListTailBytes       = 11
	petPotentialTailOffset = 3
	petListHP              = 8
	petListSP              = 12
	petListINT             = 14
	petListSTR             = 16
	petListCON             = 18
	petListAGI             = 20
	petListWIS             = 22
	petListLevel           = 7
	petListEXP             = 3
	petListPoints          = 27
	petListAmity           = 25
	petRebirthTailOffset   = 2
	petJobTailOffset       = 4
)

// UsePet retains only the authoritative roster fields needed by item targeting.
type UsePet struct {
	Skills    [3]game.PetSkill
	ID        uint16
	Name      []byte
	Potential byte
	Stats     world.Stats
	Equipment game.Equipment
	Amity     byte
}

// ApplyPetList is AC15:8 (FUN_0040b1e0). Records have variable-length names;
// validate the entire list before replacing the roster.
func (s *State) ApplyPetList(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandPetControl || p[1] != protocol.PetControlWireCode8 {
		return false
	}
	var pets [game.MaxPets]UsePet
	for o := 2; o < len(p); {
		if len(p)-o < petListPrefixBytes {
			return false
		}
		r := p[o:]
		slot := r[0]
		nameLen := int(r[petNameLengthOffset])
		size := petListPrefixBytes + nameLen + petSkillBytes + petEquipmentBytes + petListTailBytes
		if slot < 1 || slot > game.MaxPets || pets[slot-1].ID != 0 || len(r) < size || nameLen > 16 {
			return false
		}
		id := binary.LittleEndian.Uint16(r[1:])
		potential := r[size-petListTailBytes+petPotentialTailOffset]
		if id == 0 || potential > game.PotentialMaximum {
			return false
		}
		pets[slot-1] = UsePet{ID: id, Name: append([]byte(nil), r[petListPrefixBytes:petListPrefixBytes+nameLen]...), Potential: potential}
		pet := &pets[slot-1]
		pet.Stats = world.Stats{HP: binary.LittleEndian.Uint32(r[petListHP:]), SP: binary.LittleEndian.Uint16(r[petListSP:]), INT: binary.LittleEndian.Uint16(r[petListINT:]), STR: binary.LittleEndian.Uint16(r[petListSTR:]), CON: binary.LittleEndian.Uint16(r[petListCON:]), AGI: binary.LittleEndian.Uint16(r[petListAGI:]), WIS: binary.LittleEndian.Uint16(r[petListWIS:]), Level: r[petListLevel], EXP: binary.LittleEndian.Uint32(r[petListEXP:]), Points: binary.LittleEndian.Uint16(r[petListPoints:]), Potential: potential, Rebirth: r[size-petListTailBytes+petRebirthTailOffset], Job: r[size-petListTailBytes+petJobTailOffset]}
		pet.Amity = r[petListAmity]
		for i := range pet.Skills {
			at := petListPrefixBytes + nameLen + i*petSkillRecordBytes
			if r[at] > s.petSkillGradeLimit(pet.ID, i) {
				return false
			}
			pet.Skills[i] = game.PetSkill{Grade: r[at], Exp: binary.LittleEndian.Uint32(r[at+1:])}
		}
		equipment := petListPrefixBytes + nameLen + petSkillBytes
		for i := range pet.Equipment {
			at := equipment + i*equipmentRecordBytes
			id := binary.LittleEndian.Uint16(r[at:])
			if id != 0 {
				pet.Equipment[i] = game.Item{ID: id, Count: 1, Damage: r[at+2]}
				pet.Equipment[i].Metadata[game.ForgeMetadataOffset] = r[at+equipmentForgeOffset]
			}
		}
		s.recomputePet(pet)

		o += size
	}
	s.Pets = pets
	return true
}
