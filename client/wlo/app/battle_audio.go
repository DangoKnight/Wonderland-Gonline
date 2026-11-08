package app

import (
	"fmt"
	"math/rand/v2"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/clientfs"
)

const (
	battleSoundIDModulus       = 10000
	battleVoicePlayerPrefix    = "10"
	battleVoiceCompanionPrefix = "30"
	battleNativeMaximumBody    = 4
	battleNativeHumanMoveLimit = 3
	battleVoiceType7           = 7 // Native voice group; skill-type meaning unresolved.
	battleVoiceTypeSupport     = 8
	battleVoiceType10          = 10 // Native companion voice group.
	battleVoiceRolls           = 100
	// Native voice roulette cutoffs, FUN_003800a4 (rolls are 1..100).
	battleVoiceRare         = 10
	battleVoiceAttackSecond = 30
	battleVoiceAttackLast   = 60
	battleVoiceCommonSecond = 50
	battleVoiceCommonFirst  = 20
	// Native type-zero skill range and support/capture aliases.
	battleVoiceFirstBasic       = 10001
	battleVoiceLastBasic        = 10007
	battleVoiceSupportAlias     = 10024
	battleVoiceAttackAlias      = 10026
	battleWeaponEquipmentSlot   = 3
	battleVoiceNPCPrefix        = "20"
	battleVoiceNPCPrefix25      = "25"
	battleVoiceNPCPrefix25First = 31025 // FUN_00485760's template range.
	battleVoiceNPCPrefix25Last  = 31041
	battleVictoryVoiceRare      = 10
	battleVictoryVoiceSecond    = 30
	battleVictoryVoiceLast      = 60
)

func (c *Client) battleVoiceRandom(n int) int {
	if c.battle.voiceRandom != nil {
		return c.battle.voiceRandom(n)
	}
	return rand.IntN(n)
}

func (c *Client) recordedBattleVoice(prefix string, actor byte, suffix string) string {
	if prefix == "" || actor == 0 || suffix == "" {
		return ""
	}
	name := fmt.Sprintf("%s%02d%s.ogg", prefix, actor, suffix)
	for _, archive := range []string{"odd", "odd_d01"} {
		if _, err := clientfs.Stat(c.Assets.DataPath("audio", archive, name)); err == nil {
			return fmt.Sprintf(`..\audio\%s\%s`, archive, name)
		}
	}
	return ""
}

// FUN_00395f84, AC11:12: choose one voiced living fighter on the winning
// side, but speak only if the local player is also a living winner. Native
// strings 0x396594/0x3965a4/0x3965b4 are _bta3/_bta2/_bta1 (10/20/30%).
func battleVictoryVoiceSuffix(roll int) string {
	switch {
	case roll <= battleVictoryVoiceRare:
		return "_bta3"
	case roll <= battleVictoryVoiceSecond:
		return "_bta2"
	case roll <= battleVictoryVoiceLast:
		return "_bta1"
	default:
		return ""
	}
}

func (c *Client) battleVictoryVoiceTick(state clientbattle.State) {
	if state.Spectating {
		return
	}
	v := &c.battle
	if v.victorySide == 0 || v.victoryVoiced || len(v.playing) > 0 || len(state.Actions) > 0 || len(v.overkills) > 0 {
		return
	}
	v.victoryVoiced = true
	if c.World == nil || c.Env.Sound == nil {
		return
	}
	localWinner := false
	var candidates []clientbattle.Fighter
	for _, f := range state.Fighters {
		if f.Side != v.victorySide || f.HP == 0 || f.Overkilled {
			continue
		}
		if f.Kind == clientbattle.Player && f.ID == state.Self {
			localWinner = true
		}
		_, prefix, actor := c.battleAudioActor(f)
		if prefix != "" && actor != 0 {
			candidates = append(candidates, f)
		}
	}
	if !localWinner || len(candidates) == 0 {
		return
	}
	roll := c.battleVoiceRandom(battleVoiceRolls) + 1
	f := candidates[c.battleVoiceRandom(len(candidates))]
	_, prefix, actor := c.battleAudioActor(f)
	if path := c.recordedBattleVoice(prefix, actor, battleVictoryVoiceSuffix(roll)); path != "" {
		c.Env.Sound(path)
	}
}

// newBattlePlay prepares FUN_003800a4's three sound channels once per actor.
// General sounds coexist with weapon sounds; a recorded line replaces body
// sounds. Immutable motion tables remain shared between workspace sessions.
func (c *Client) newBattlePlay(a clientbattle.Action, at time.Time, soundSource clientbattle.Cell) battlePlay {
	p := battlePlay{action: a, at: at}
	if c.battle.assets == nil {
		return p
	}
	m := c.battle.assets.Motions[a.Skill]
	p.audio = append(p.audio, m.Sounds...)
	f := c.battle.state.At(a.Actor)
	if f == nil {
		return p
	}
	body, prefix, actor := c.battleAudioActor(*f)
	if len(a.Targets) > 0 {
		p.voice = c.battleVoice(a, prefix, actor, body)
	}
	weapon := c.battleWeaponSoundGroup(soundSource)
	for channel := 1; channel < clientbattle.ActorSoundChannels; channel++ {
		if int(weapon) < len(m.WeaponSounds) && m.WeaponSounds[weapon][channel]%battleSoundIDModulus != 0 {
			p.audio = append(p.audio, clientbattle.SoundCue{ID: m.WeaponSounds[weapon][channel], Trigger: m.WeaponSoundTriggers[channel]})
		}
		if p.voice == "" && int(body) < len(m.BodySounds) && m.BodySounds[body][channel]%battleSoundIDModulus != 0 {
			p.audio = append(p.audio, clientbattle.SoundCue{ID: m.BodySounds[body][channel], Trigger: m.BodySoundTriggers[channel]})
		}
	}
	return p
}

// FUN_0037c660's weapon resolver, recovered from the original executable
// at 0x37c878..0x37ccc1. The C decompiler incorrectly removed this block.
func nativeWeaponSoundGroup(kind byte) byte {
	switch kind {
	case 1, 2, 3, 6, 8:
		return 1
	case 0, 7, 9:
		return 2
	case 5, 10:
		return 3
	case 4:
		return 4
	case 11:
		return 5
	default:
		return 0 // Native unresolved/unsupported item category.
	}
}

func (c *Client) battleWeaponSoundGroup(cell clientbattle.Cell) byte {
	f := c.battle.state.At(cell)
	if f == nil {
		return 0
	}
	if f.Kind != clientbattle.Player {
		t := c.npcTemplates[f.ID]
		if t.MoveKind >= battleNativeHumanMoveLimit {
			return 0
		}
		if t.MoveKind == 1 {
			return nativeWeaponSoundGroup(t.BattleSoundWeapon)
		}
		if f.Kind == clientbattle.Pet && f.Owner == c.World.Player.ID {
			for _, p := range c.InventoryState.Pets {
				if uint32(p.ID) == f.ID {
					return nativeWeaponSoundGroup(c.items[p.Equipment[battleWeaponEquipmentSlot-1].ID].Definition.Type)
				}
			}
		}
		return nativeWeaponSoundGroup(0)
	}
	p := c.World.Player
	if f.ID != p.ID {
		if peer := c.World.Peers[f.ID]; peer != nil {
			p = peer.Player
		} else if peer := c.teamAppearances[f.ID]; peer != nil {
			p = peer.Player
		} else {
			return 0
		}
	}
	for _, id := range p.Items {
		if it, ok := c.items[id]; ok && it.Definition.EquipSlot == battleWeaponEquipmentSlot {
			return nativeWeaponSoundGroup(it.Definition.Type)
		}
	}
	return nativeWeaponSoundGroup(0)
}

func (c *Client) battleAudioActor(f clientbattle.Fighter) (body byte, prefix string, actor byte) {
	if f.Kind == clientbattle.Player {
		p := c.World.Player
		if f.ID != p.ID {
			if peer := c.World.Peers[f.ID]; peer != nil {
				p = peer.Player
			} else if peer := c.teamAppearances[f.ID]; peer != nil {
				p = peer.Player
			} else {
				return
			}
		}
		return p.Body, battleVoicePlayerPrefix, playerVoiceActors[[2]byte{p.Body, p.Head}]
	}
	t := c.npcTemplates[f.ID]
	if t.MoveKind >= battleNativeHumanMoveLimit {
		return
	}
	body = t.BattleSoundBody
	if t.Voice > 0 && t.Voice <= voiceNPCMaximum {
		prefix = battleVoiceNPCPrefix
		if f.ID >= battleVoiceNPCPrefix25First && f.ID <= battleVoiceNPCPrefix25Last {
			prefix = battleVoiceNPCPrefix25
		}
		return body, prefix, t.Voice
	}
	return body, battleVoiceCompanionPrefix, companionVoiceActors[t.Voice]
}

// battleVoiceSuffix follows the native roulette, including its silent basic
// attack rolls. The roll is cosmetic and never affects server battle state.
func battleVoiceSuffix(d skills.Definition, roll int, self, ally, companion bool) string {
	if !companion && d.Type == battleVoiceType7 || companion && d.Type == battleVoiceType10 {
		if roll <= battleVoiceRare {
			return "_bt73"
		}
		if roll <= battleVoiceCommonSecond {
			return "_bt72"
		}
		return "_bt71"
	}
	if d.Type == battleVoiceTypeSupport || d.Type == 0 && d.ID == battleVoiceSupportAlias {
		if self {
			return "_bt81"
		}
		if ally {
			return "_bt82"
		}
		return "_bt83"
	}
	if d.Type == 0 {
		if (d.ID < battleVoiceFirstBasic || d.ID > battleVoiceLastBasic) && d.ID != battleVoiceAttackAlias {
			return ""
		}
		if roll <= battleVoiceRare {
			return "_bt43"
		}
		if roll <= battleVoiceAttackSecond {
			return "_bt42"
		}
		if roll <= battleVoiceAttackLast {
			return "_bt41"
		}
		return ""
	}
	if roll <= battleVoiceCommonFirst {
		return "_bt63"
	}
	if roll <= battleVoiceCommonSecond {
		return "_bt62"
	}
	return "_bt61"
}

func (c *Client) battleVoice(a clientbattle.Action, prefix string, actor, body byte) string {
	if c.SkillState == nil || c.SkillState.Catalog == nil {
		return ""
	}
	d, known := c.SkillState.Catalog.Definitions[a.Skill]
	if !known {
		return ""
	}
	target := battleTarget(a)
	suffix := battleVoiceSuffix(d, c.battleVoiceRandom(battleVoiceRolls)+1, a.Actor == target, (a.Actor.X > 2) == (target.X > 2), prefix != battleVoicePlayerPrefix)
	if path := c.recordedBattleVoice(prefix, actor, suffix); path != "" {
		return path
	}
	// Native Voice\<body family>_<skill voice ID>.WAV fallback. The original
	// family mapping ignores head variants (FUN_004854d8).
	if body > 0 && body <= battleNativeMaximumBody && d.BattleVoice != 0 {
		family := [...]string{"", "km", "kw", "am", "aw"}[body]
		name := fmt.Sprintf("%s_%03d.wav", family, d.BattleVoice)
		if _, err := clientfs.Stat(audioAssetPath(c.Assets.MediaPath("voice", name))); err == nil {
			return `voice\` + name
		}
	}
	return ""
}

func (c *Client) battleAudio(p *battlePlay, m clientbattle.Motion, now time.Time) {
	if !p.voiced {
		p.voiced = true
		if p.voice != "" && c.Env.Sound != nil {
			c.Env.Sound(p.voice)
		}
	}
	for i, sound := range p.audio {
		mask := uint64(1) << i
		if p.sounds&mask == 0 && now.Sub(p.at) >= m.EventAt(clientbattle.MotionEvent{Trigger: sound.Trigger}) {
			p.sounds |= mask
			if id := sound.ID % battleSoundIDModulus; id != 0 && c.Env.Sound != nil {
				c.Env.Sound(fmt.Sprintf(`sound\SEB%04d.wav`, id))
			}
		}
	}
}
