package server

import (
	"context"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

func (s *Server) combatRules(run *battleRun) battle.Rules {
	if run.items == nil {
		run.items = s.battleItems()
	}
	r := s.rules()
	r.Items = run.items
	r.CommitCapture = func(catch battle.Capture) (battle.Step, bool) { return s.commitCombatCapture(run, catch) }
	r.RewardKill = func(target *battle.Fighter, actors []battle.Action) battle.Step {
		return s.commitCombatLoot(run, target, actors)
	}
	r.PrepareCapture = func(a battle.Action) (int, bool) {
		// No PvP mode may consume equipment durability, even if a future caller
		// reaches this hook without the engine's normal capture rejection.
		if run.b.PvP {
			return 0, false
		}
		for _, member := range run.active() {
			if member.self.ID != a.Player {
				continue
			}
			c := member.c
			expected, ok := c.character.CaptureNet()
			if !ok {
				return game.OrdinaryCaptureLevelAllowance, true
			}
			var saved game.Character
			ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
			err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
				net, valid := stored.CaptureNet()
				if !valid || net != expected {
					return game.ErrInvalidItem
				}
				stored.Equipment[game.CaptureNetEquipmentSlot-1].Damage = min(game.CaptureNetExhaustedDamage, net.Damage+game.CaptureNetDamagePerUse)
				if err := game.PreserveItemLocks(*c.character, stored); err != nil {
					return err
				}
				saved = stored.Clone()
				return nil
			})
			cancel()
			if err != nil {
				return 0, false
			}
			s.adoptSavedCharacter(c, saved)
			_ = c.send(c.character.EquipmentPacket())
			return game.CaptureNetLevelAllowance, true
		}
		return 0, false
	}
	r.CommitItem = func(a battle.Action, t *battle.Fighter, hp, sp int) bool {
		return s.commitBattleItem(run, a, t, hp, sp)
	}
	return r
}

func (s *Server) battleItems() map[uint16]assets.BattleItem {
	items := make(map[uint16]assets.BattleItem)
	for id, d := range s.Assets.Items {
		native := s.Assets.NativeItems[id]
		native.Definition = d
		if info, ok := native.BattleItem(); ok {
			items[id] = info
		}
	}
	return items
}

// Round owns worldMu. Consumption and the target's recoverable vitals are one
// SQL transaction, even for another player's pet. Never publish failed saves.
func (s *Server) commitBattleItem(run *battleRun, a battle.Action, target *battle.Fighter, hp, sp int) bool {
	var owner, recipient *Session
	for _, m := range run.active() {
		if m.self.ID == a.Player {
			owner = m.c
		}
		if m.self == target || m.pet == target {
			recipient = m.c
		}
	}
	if owner == nil || a.ItemSlot < 1 || a.ItemSlot > game.BagSize {
		return false
	}
	if recipient == nil && (target.Kind == battle.Player || target.Kind == battle.Pet) {
		return false
	}
	expected := owner.character.Bag[a.ItemSlot-1]
	if expected.Empty() || expected.Locked || expected.ID != a.Item.ID || expected.Damage != a.Item.Damage || expected.Metadata != a.Item.Metadata {
		return false
	}
	var savedOwner, savedRecipient game.Character
	apply := func(from, to *game.Character) error {
		if from.Bag[a.ItemSlot-1] != expected {
			return game.ErrInvalidItem
		}
		if err := from.Bag.Remove(a.ItemSlot, 1); err != nil {
			return err
		}
		if to != nil {
			if target.Kind == battle.Player {
				to.HP, to.SP = uint32(hp), uint32(sp)
			} else {
				found := false
				for i := range to.Pets {
					if uint32(to.Pets[i].ID) == target.ID {
						to.Pets[i].HP, to.Pets[i].SP = int32(hp), int32(sp)
						found = true
						break
					}
				}
				if !found {
					return game.ErrInvalidItem
				}
			}
			if recipient != owner {
				if err := game.PreserveItemLocks(*recipient.character, to); err != nil {
					return err
				}
			}
			savedRecipient = to.Clone()
		}
		if err := game.PreserveItemLocks(*owner.character, from); err != nil {
			return err
		}
		savedOwner = from.Clone()
		return nil
	}
	ref := store.CharacterRef{Account: owner.account.ID, ID: owner.character.ID}
	ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
	defer cancel()
	var err error
	if recipient != nil && recipient != owner {
		err = s.Store.UpdateCharacterPair(ctx, ref, store.CharacterRef{Account: recipient.account.ID, ID: recipient.character.ID}, apply)
	} else {
		err = s.Store.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
			if recipient == owner {
				return apply(c, c)
			}
			return apply(c, nil)
		})
	}
	if err != nil {
		s.Log.Warn("battle item not committed", "character", a.Player, "error", err)
		return false
	}
	s.adoptSavedCharacter(owner, savedOwner)
	if recipient != nil && recipient != owner {
		s.adoptSavedCharacter(recipient, savedRecipient)
	}
	// An explicit battle result takes precedence over buffered resting vitals,
	// even when it happens to equal the previous checkpoint.
	if recipient != nil {
		if target.Kind == battle.Player {
			recipient.character.HP, recipient.character.SP = uint32(hp), uint32(sp)
		} else {
			for i := range recipient.character.Pets {
				if uint32(recipient.character.Pets[i].ID) == target.ID {
					recipient.character.Pets[i].HP, recipient.character.Pets[i].SP = int32(hp), int32(sp)
				}
			}
		}
	}
	return true
}
