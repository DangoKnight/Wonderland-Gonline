package inventory

import "wonderland-gonline/internal/localization"

import (
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	compoundJuniorX    = 21
	compoundJuniorY    = 50
	compoundJuniorSize = 16
	compoundTierX      = 56
	compoundTierY      = 47
	compoundTierWidth  = 43
	compoundTierHeight = 20
	compoundTierMenuY  = -15
)

// FUN_0021dd44/0021de54: Junior checkbox or Superior tier selector.
// A sole learned tier is automatic; choices never learn a skill.
func (f *CompoundForm) initTierSelector() {
	f.Junior = f.button("Btn_UnCheck_1", compoundJuniorX, compoundJuniorY, compoundJuniorSize, compoundJuniorSize, func() {
		if f.Tier == game.AlchemyJunior {
			f.SelectTier(game.AlchemyPrimary)
		} else {
			f.SelectTier(game.AlchemyJunior)
		}
	})
	f.Junior.BindLanguage("hint", func() { f.Junior.SetHint([]byte(localization.UI("ui.inventory.use_junior_alchemy_43f5ad51"))) })
	f.TierButton = f.button("Btn_CompSk_1", compoundTierX, compoundTierY, compoundTierWidth, compoundTierHeight, func() {
		if f.allowed() {
			f.tierMenu = !f.tierMenu
			f.refreshTierSelector()
		}
	})
	f.TierButton.BindLanguage("hint", func() { f.TierButton.SetHint([]byte(localization.UI("ui.inventory.choose_alchemy_skill_65ae7f41"))) })
	for i := range f.TierOptions {
		tier := game.AlchemyTier(i)
		f.TierOptions[i] = f.button(compoundTierArt(tier), compoundTierX, compoundTierMenuY+(len(f.TierOptions)-1-i)*compoundTierHeight, compoundTierWidth, compoundTierHeight, func() { f.SelectTier(tier) })
		f.TierOptions[i].BindLanguage("hint", func() {
			f.TierOptions[i].SetHint([]byte([]string{localization.UI("ui.inventory.primary_alchemy_f3aa477d"), localization.UI("ui.inventory.junior_alchemy_46fa2bf0"), localization.UI("ui.inventory.superior_alchemy_5b432b27")}[i]))
		})
	}
}
func compoundTierArt(tier game.AlchemyTier) string {
	switch tier {
	case game.AlchemyJunior:
		return "Btn_CompSk_2"
	case game.AlchemySuperior:
		return "Btn_CompSk_3"
	}
	return "Btn_CompSk_1"
}
func (f *CompoundForm) learned(id uint16) bool { return f.SkillLearned != nil && f.SkillLearned(id) }
func (f *CompoundForm) tierAvailable(tier game.AlchemyTier) bool {
	switch tier {
	case game.AlchemyPrimary:
		return f.learned(game.AlchemyPrimarySkill) || f.learnedTierCount() == 0
	case game.AlchemyJunior:
		return f.learned(game.AlchemyJuniorSkill)
	case game.AlchemySuperior:
		return f.learned(game.AlchemySuperiorSkill)
	}
	return false
}
func (f *CompoundForm) learnedTierCount() int {
	count := 0
	for _, id := range []uint16{game.AlchemyPrimarySkill, game.AlchemyJuniorSkill, game.AlchemySuperiorSkill} {
		if f.learned(id) {
			count++
		}
	}
	return count
}
func (f *CompoundForm) normalizeTier() {
	if f.tierAvailable(f.Tier) {
		return
	}
	for tier := game.AlchemyPrimary; tier <= game.AlchemySuperior; tier++ {
		if f.tierAvailable(tier) {
			f.Tier = tier
			return
		}
	}
}
func (f *CompoundForm) SelectTier(tier game.AlchemyTier) bool {
	if !f.allowed() || !f.tierAvailable(tier) {
		return false
	}
	f.Tier = tier
	f.tierMenu = false
	f.refreshTierSelector()
	return true
}
func (f *CompoundForm) AlchemyCommand() byte {
	f.normalizeTier()
	switch f.Tier {
	case game.AlchemyJunior:
		return protocol.InventoryCompoundJunior
	case game.AlchemySuperior:
		return protocol.InventoryCompoundSuperior
	}
	return protocol.InventoryCompound
}
func (f *CompoundForm) refreshTierSelector() {
	if f.Junior == nil {
		return
	}
	f.normalizeTier()
	choice := f.learnedTierCount() > 1
	superior := choice && f.learned(game.AlchemySuperiorSkill)
	if !superior {
		f.tierMenu = false
	}
	f.Junior.Visible = choice && !superior && f.learned(game.AlchemyJuniorSkill)
	f.Junior.Enabled = f.allowed()
	art := "Btn_UnCheck_1"
	if f.Tier == game.AlchemyJunior {
		art = "Btn_Check_1"
	}
	f.Junior.Image = f.Env.Pics.Find(art)
	f.TierButton.Visible = superior
	f.TierButton.Enabled = f.allowed()
	f.TierButton.Image = f.Env.Pics.Find(compoundTierArt(f.Tier))
	for i, b := range f.TierOptions {
		b.Visible = superior && f.tierMenu && f.tierAvailable(game.AlchemyTier(i))
		b.Enabled = f.allowed() && f.tierAvailable(game.AlchemyTier(i))
	}
}
