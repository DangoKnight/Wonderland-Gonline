package assets

import (
	"fmt"
	"math"
	"slices"
	"wonderland-gonline/internal/game"
)

const (
	MaxStoryAutomaticTransitions = 128
	maxStoryRules                = 16384
	maxStoryIdentityBytes        = 128
	maxStoryRegionMargin         = 16
)

func ValidateStoryDefinitions(d StoryDefinitions) error {
	if err := validateStoryStructure(d); err != nil {
		return err
	}
	if len(d.Rules) > maxStoryRules {
		return fmt.Errorf("too many story rules")
	}
	scope := func(s StoryScope) error {
		if len(s.Maps) == 0 && len(s.Events) > 0 && !s.AllowMissing {
			return fmt.Errorf("event scope requires a map")
		}
		for _, list := range [][]uint16{s.Maps, s.Events, s.Actors} {
			seen := map[uint16]bool{}
			for _, id := range list {
				if id == 0 || seen[id] {
					return fmt.Errorf("zero/duplicate story scope identity")
				}
				seen[id] = true
			}
		}
		seen := map[byte]bool{}
		for _, id := range s.Branches {
			if id == 0 || seen[id] {
				return fmt.Errorf("zero/duplicate story branch")
			}
			seen[id] = true
		}
		return nil
	}
	marks := func(rows []StoryMark) error {
		seen := map[uint32]bool{}
		for _, m := range rows {
			if m.ID == 0 || seen[m.ID] || m.Value < 0 || m.Value > math.MaxUint8 {
				return fmt.Errorf("invalid story mark")
			}
			seen[m.ID] = true
			switch m.Mode {
			case "complete", "advance", "ensure_active", "set":
			default:
				return fmt.Errorf("unknown story mark mode %q", m.Mode)
			}
		}
		return nil
	}
	equipment := func(rows []StoryEquipment) error {
		seen := map[byte]bool{}
		for _, r := range rows {
			if r.ItemID == 0 || int(r.Slot) >= len(game.Equipment{}) || seen[r.Slot] {
				return fmt.Errorf("invalid story equipment")
			}
			seen[r.Slot] = true
		}
		return nil
	}
	for _, r := range d.Rules {
		if len(r.ID) > maxStoryIdentityBytes {
			return fmt.Errorf("story identity too long")
		}
		if err := scope(r.Scope); err != nil {
			return fmt.Errorf("story %s: %w", r.ID, err)
		}
		switch r.Kind {
		case StoryVisibility, StoryFinalVisibility, StoryCondition, StoryPreCondition, StoryPropCondition:
			if r.Value < 0 || r.Value > 1 {
				return fmt.Errorf("story %s: expected boolean result", r.ID)
			}
		case StoryPriority:
			if r.Value < 1 || r.Value > math.MaxUint8 {
				return fmt.Errorf("invalid story priority")
			}
		case StoryRegionMargin:
			if r.Value < 0 || r.Value > maxStoryRegionMargin {
				return fmt.Errorf("invalid story region margin")
			}
		case StoryEquipmentGate:
			if r.Value < 0 || r.Value >= int64(len(game.Equipment{})) {
				return fmt.Errorf("invalid story equipment slot")
			}
		case StoryContinue:
			if r.TargetEvent == 0 && r.TargetBranch == 0 && len(r.Scope.Events) == 0 {
				return fmt.Errorf("continuation requires an event scope or target")
			}
		case StoryClickMark:
			if r.Mark == 0 || r.Value < 0 || r.Value > math.MaxUint8 {
				return fmt.Errorf("invalid story checkpoint")
			}
		case StoryTransition:
			if r.Destination.Map == 0 {
				return fmt.Errorf("invalid story destination")
			}
		case StoryOutcome:
			if r.Value != StoryOutcomeForward && r.Value != StoryOutcomeBounded {
				return fmt.Errorf("invalid story battle callback policy")
			}
		}
		for _, g := range r.Gates {
			if g.Group > math.MaxUint8 || g.Source == "equipment" && int(g.Slot) >= len(game.Equipment{}) {
				return fmt.Errorf("invalid story gate")
			}
		}
		for _, cue := range r.Cues {
			if err := validateStoryCue(cue); err != nil {
				return err
			}
		}
	}
	seenTemplates := map[uint32]bool{}
	for _, r := range d.Recruitments {
		if err := scope(r.Scope); err != nil {
			return err
		}
		if err := marks(r.Marks); err != nil {
			return err
		}
		if r.MovieMode < 1 || r.MovieMode > 2 || seenTemplates[r.Template] || len(r.Marks) == 0 {
			return fmt.Errorf("invalid recruitment")
		}
		seenTemplates[r.Template] = true
		completed := false
		for _, m := range r.Marks {
			completed = completed || m.ID == r.CompletedMark && m.Mode == "complete"
		}
		if !completed {
			return fmt.Errorf("recruitment lacks completion checkpoint")
		}
		for _, lines := range [][]StoryLine{r.Lines, r.RepeatLines, r.FullPartyLines} {
			for _, l := range lines {
				if l.Talk == 0 || l.Talk > 0xffffff || l.Portrait != 3 && l.Portrait != 7 {
					return fmt.Errorf("invalid recruitment dialogue")
				}
			}
		}
	}
	for _, r := range d.Transformations {
		if err := scope(r.Scope); err != nil {
			return err
		}
		if err := marks(r.Marks); err != nil {
			return err
		}
		if len(game.StarterOutfit(r.Body, r.Head)) == 0 || len(r.Program) == 0 || len(r.Scope.Branches) == 0 || r.Pattern.Code == nil {
			return fmt.Errorf("invalid transformation guard")
		}
		for _, b := range r.Program {
			if b.Index == 0 || b.ExtraConditions < -1 || b.ExtraConditions > math.MaxUint8 {
				return fmt.Errorf("invalid transformation program guard")
			}
		}
	}
	for _, r := range d.Rewards {
		if err := scope(r.Scope); err != nil {
			return err
		}
		if err := marks(r.Marks); err != nil {
			return err
		}
		for _, item := range r.ExtraItems {
			if item.ItemID == 0 || item.Count < 1 || item.Count > math.MaxInt32 {
				return fmt.Errorf("invalid story reward item")
			}
		}
		checkpoint := false
		for _, m := range r.Marks {
			checkpoint = checkpoint || m.ID == r.CompletedMark && m.Mode == "complete"
		}
		if !checkpoint {
			return fmt.Errorf("reward lacks completion checkpoint")
		}
	}
	seenQuests := map[uint32]bool{}
	for _, r := range d.BattleRewards {
		if r.Quest == 0 || seenQuests[r.Quest] || r.Gold > game.MaxGold || len(r.Monsters) == 0 && len(r.Aliases) == 0 {
			return fmt.Errorf("invalid battle reward")
		}
		seenQuests[r.Quest] = true
		if err := equipment(r.Equipment); err != nil {
			return err
		}
	}
	for from, to := range d.CompanionAliases {
		if from == 0 || to == 0 || from == to {
			return fmt.Errorf("invalid companion alias")
		}
		if _, chained := d.CompanionAliases[to]; chained {
			return fmt.Errorf("chained companion aliases are unsupported")
		}
	}
	for _, r := range d.CompanionEquipment {
		if r.Template == 0 {
			return fmt.Errorf("invalid companion template")
		}
		if err := equipment(r.Equipment); err != nil {
			return err
		}
	}
	ids := map[string]bool{}
	roles := map[string]map[uint16]bool{}
	for _, r := range d.Cinematics {
		if r.ID == "" || ids[r.ID] || len(r.Steps) == 0 || len(r.Maps) == 0 {
			return fmt.Errorf("invalid cinematic identity/steps")
		}
		ids[r.ID] = true
		if r.Role != "arrival" && r.Role != "movie_handshake" {
			return fmt.Errorf("unknown cinematic role %q", r.Role)
		}
		if roles[r.Role] == nil {
			roles[r.Role] = map[uint16]bool{}
		}
		for _, id := range r.Maps {
			if id == 0 || roles[r.Role][id] {
				return fmt.Errorf("ambiguous cinematic role/map")
			}
			roles[r.Role][id] = true
		}
		if r.Role == "movie_handshake" && (r.DestinationMap == 0 || r.Trigger == (StoryPattern{})) {
			return fmt.Errorf("missing movie destination/trigger")
		}
		if r.Role == "arrival" && r.CompletionMark == 0 {
			return fmt.Errorf("missing arrival completion mark")
		}
		for _, step := range r.Steps {
			for _, cue := range step.Cues {
				if err := validateStoryCue(cue); err != nil {
					return err
				}
			}
		}
		for _, cue := range r.Finish {
			if err := validateStoryCue(cue); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStoryCue(c StoryCue) error {
	counts := map[string]int{"movie": 4, "pose": 1, "character_state": 2, "movie_handshake": 5, "actor_movement": 2, "actor_action": 4, "quest_state": 2, "quest_flag": 2, "movement_lock": 1, "event_resume": 0, "event_close": 0, "step_complete": 0, "character_refresh": 0, "territory_empty": 1}
	n, ok := counts[c.Kind]
	if !ok || len(c.Data) != n || !c.Self && !c.Broadcast {
		return fmt.Errorf("invalid cinematic cue %q", c.Kind)
	}
	for i, v := range c.Data {
		limit := uint32(math.MaxUint8)
		if c.Kind == "movie" && i == 1 {
			limit = math.MaxUint32
		}
		if c.Kind == "actor_movement" || ((c.Kind == "quest_state" || c.Kind == "quest_flag") && i == 0) {
			limit = math.MaxUint16
		}
		if v > limit {
			return fmt.Errorf("cinematic %s operand exceeds wire range", c.Kind)
		}
	}
	if c.Kind == "movie" && (c.Data[0] < 1 || c.Data[0] > 2) {
		return fmt.Errorf("invalid cinematic movie mode")
	}
	return nil
}

// Compatibility seeds may refer to absent native content in partial catalogs.
// Newly authored scopes default to strict reference checking; operators can
// explicitly retain dormant compatibility rows with allow_missing.
func ValidateStoryCatalog(c *Catalog) error {
	if err := ValidateStoryDefinitions(c.Story); err != nil {
		return err
	}
	scopes := append([]StoryRule(nil), c.Story.Rules...)
	for _, r := range c.Story.Recruitments {
		scopes = append(scopes, StoryRule{ID: r.ID, Scope: r.Scope})
	}
	for _, r := range c.Story.Transformations {
		scopes = append(scopes, StoryRule{ID: r.ID, Scope: r.Scope, Gates: r.Gates})
	}
	for _, r := range c.Story.Rewards {
		scopes = append(scopes, StoryRule{ID: r.ID, Scope: r.Scope})
	}
	for _, r := range scopes {

		if r.Scope.AllowMissing {
			continue
		}
		for _, g := range r.Gates {
			if g.Source == "mark_state" || g.Source == "mark_step" || g.Source == "active_mark_step" || g.Source == "mark_exists" {
				_, native := c.Marks[uint16(g.ID)]
				_, custom := c.QuestDefinitions[g.ID]
				if g.ID == 0 || (!custom && !(g.ID <= math.MaxUint16 && native)) {
					return fmt.Errorf("story %s: unknown mark %d", r.ID, g.ID)
				}
			}
		}
		for _, id := range r.Scope.Maps {
			m, ok := c.Maps[id]
			if !ok {
				return fmt.Errorf("story %s: unknown map %d", r.ID, id)
			}
			events := m.Events
			if r.Kind == StoryPreFallback {
				events = m.PreEvents
			}
			for _, eid := range r.Scope.Events {
				var ev *Event
				for i := range events {
					if events[i].ClickID == eid {
						ev = &events[i]
						break
					}
				}
				if ev == nil {
					return fmt.Errorf("story %s: unknown event %d", r.ID, eid)
				}
				for _, b := range r.Scope.Branches {
					if !slices.ContainsFunc(ev.Branches, func(br Branch) bool { return br.Index == b }) {
						return fmt.Errorf("story %s: unknown branch %d", r.ID, b)
					}
				}
			}
			if r.Kind == StoryTransition {
				if _, ok := c.Maps[r.Destination.Map]; !ok {
					return fmt.Errorf("story %s: unknown destination", r.ID)
				}
			}
			if r.TargetEvent != 0 {
				switch r.Kind {
				case StoryTransition:
					if _, ok := c.Maps[r.TargetEvent]; !ok {
						return fmt.Errorf("story %s: unknown destination", r.ID)
					}
				case StoryContinue, StoryCandidate, StoryClick, StoryArrival, StoryArrivalCleanup:
					target := m.Events
					if r.Kind == StoryArrival && r.UsePreEvent {
						target = m.PreEvents
					}
					if !slices.ContainsFunc(target, func(e Event) bool { return e.ClickID == r.TargetEvent }) {
						return fmt.Errorf("story %s: unknown target event", r.ID)
					}
				}
			}
			for _, actor := range append(append([]uint16(nil), r.Scope.Actors...), r.TargetActors...) {
				exists := slices.ContainsFunc(m.NPCs, func(n MapNPC) bool { return n.ClickID == actor })
				for _, ev := range append(append([]Event(nil), m.Events...), m.PreEvents...) {
					for _, br := range ev.Branches {
						for _, o := range br.Operations {
							exists = exists || o.Data[0] == 2 && uint16(o.Data[1])|uint16(o.Data[2])<<8 == actor
						}
					}
				}
				if !exists {
					return fmt.Errorf("story %s: unknown actor %d", r.ID, actor)
				}
			}
		}
	}
	template := func(id uint32) bool {
		_, raw := c.NPCs[uint16(id)]
		_, wire := c.NPCs[uint16(game.BroadcastID(id))]
		return id == 0 || id <= math.MaxUint16 && (raw || wire)
	}
	items := func(rows []QuestItem) bool {
		for _, r := range rows {
			if _, ok := c.Items[r.ItemID]; !ok {
				return false
			}
		}
		return true
	}
	equipment := func(rows []StoryEquipment) bool {
		for _, r := range rows {
			if _, ok := c.Items[r.ItemID]; !ok {
				return false
			}
		}
		return true
	}
	for _, r := range c.Story.Recruitments {
		if !r.Scope.AllowMissing && !template(r.Template) {
			return fmt.Errorf("story %s: unknown companion", r.ID)
		}
	}
	for _, r := range c.Story.Rewards {
		if !r.Scope.AllowMissing && !items(r.ExtraItems) {
			return fmt.Errorf("story %s: unknown reward item", r.ID)
		}
	}
	for _, r := range c.Story.Transformations {
		if !r.Scope.AllowMissing {
			for _, id := range game.StarterOutfit(r.Body, r.Head) {
				if _, ok := c.Items[id]; !ok {
					return fmt.Errorf("story %s: unknown outfit item", r.ID)
				}
			}
		}
	}
	for _, r := range c.Story.BattleRewards {
		if !r.AllowMissing {
			if !template(r.Companion) || !equipment(r.Equipment) {
				return fmt.Errorf("invalid story battle reward reference")
			}
			for _, id := range r.Monsters {
				if !template(id) {
					return fmt.Errorf("unknown story battle monster")
				}
			}
		}
	}
	for _, r := range c.Story.CompanionEquipment {
		if !r.AllowMissing && (!template(r.Template) || !equipment(r.Equipment)) {
			return fmt.Errorf("unknown companion equipment reference")
		}
	}
	for _, r := range c.Story.Cinematics {
		if !r.AllowMissing {
			for _, id := range r.Maps {
				if _, ok := c.Maps[id]; !ok {
					return fmt.Errorf("cinematic %s: unknown map", r.ID)
				}
			}
			if r.DestinationMap != 0 {
				if _, ok := c.Maps[r.DestinationMap]; !ok {
					return fmt.Errorf("cinematic %s: unknown destination", r.ID)
				}
			}
		}
	}
	return nil
}
