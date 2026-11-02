package assets

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// StoryDefinitions extends imported EVE programs with editable, typed policies.
// Runtime reads the SQL projection; DefaultStoryDefinitions is an offline seed.
type StoryDefinitions struct {
	FullBagWarnings    []uint32                  `json:"full_bag_warnings"`
	PrivateScenes      []uint16                  `json:"private_scenes"`
	CompanionEquipment []StoryCompanionEquipment `json:"companion_equipment"`
	CompanionAliases   map[uint32]uint32         `json:"companion_aliases"`
	Rules              []StoryRule               `json:"rules"`
	Recruitments       []StoryRecruitment        `json:"recruitments"`
	Transformations    []StoryTransformation     `json:"transformations"`
	Rewards            []StoryReward             `json:"rewards"`
	BattleRewards      []StoryBattleReward       `json:"battle_rewards"`
	Cinematics         []StoryCinematic          `json:"cinematics"`
}

const (
	StoryForwardOnly          = "forward_only"
	StoryPriority             = "branch_priority"
	StoryBlock                = "block_event"
	StoryContinue             = "continue"
	StoryClick                = "click_route"
	StoryClickMark            = "click_mark"
	StoryCandidate            = "candidate"
	StoryArrival              = "arrival"
	StoryArrivalCleanup       = "arrival_cleanup"
	StoryArrivalRegion        = "arrival_region"
	StoryVisibility           = "visibility"
	StoryFinalVisibility      = "final_visibility"
	StoryCondition            = "condition"
	StoryPreCondition         = "precondition"
	StoryPropCondition        = "prop_condition"
	StoryEquipmentGate        = "equipment_gate"
	StoryPreFallback          = "pre_fallback"
	StoryMovieMode            = "movie_mode"
	StoryChoiceMode           = "choice_mode"
	StoryActorCapability      = "actor_capability"
	StoryOutcome              = "outcome"
	StoryRepair               = "repair"
	StoryRegionMargin         = "region_margin"
	StoryTransition           = "transition"
	StoryExchange             = "item_exchange"
	StoryRecruitBlock         = "recruit_block"
	StorySyncActors           = "sync_actors"
	StoryPreserveActor        = "preserve_actor"
	StoryArrivalPackets       = "arrival_packets"
	StoryPortalConsumeVehicle = "portal_consume_vehicle"
)

// Zero scope fields are wildcards. Multiple gates in one group are ANDed;
// groups are ORed. An empty gate list matches unconditionally.
type StoryScope struct {
	AllowMissing bool     `json:"allow_missing,omitempty"`
	Maps         []uint16 `json:"maps,omitempty"`
	Events       []uint16 `json:"events,omitempty"`
	Branches     []byte   `json:"branches,omitempty"`
	Actors       []uint16 `json:"actors,omitempty"`
}

type StoryGate struct {
	Source string `json:"source"`
	ID     uint32 `json:"id,omitempty"`
	Slot   byte   `json:"slot,omitempty"`
	Op     string `json:"op"`
	Value  int64  `json:"value"`
	Group  uint16 `json:"group,omitempty"`
}

// Nullable fields allow matching explicit zero operands. Values are decoded
// exactly like EVE; a pattern never contains executable SQL or Go expressions.
type StoryPattern struct {
	D6Low *byte   `json:"d6_low,omitempty"`
	Code  *byte   `json:"code,omitempty"`
	D1    *uint16 `json:"d1,omitempty"`
	D2    *uint16 `json:"d2,omitempty"`
	D3    *uint16 `json:"d3,omitempty"`
	D4    *uint16 `json:"d4,omitempty"`
	D5    *uint16 `json:"d5,omitempty"`
	D6    *uint16 `json:"d6,omitempty"`
	Value *uint32 `json:"value,omitempty"`
}

func (p StoryPattern) Matches(raw [21]byte) bool {
	if p.D6Low != nil && raw[11] != *p.D6Low {
		return false
	}
	if p.Code != nil && raw[0] != *p.Code {
		return false
	}
	for i, want := range []*uint16{p.D1, p.D2, p.D3, p.D4, p.D5, p.D6} {
		if want != nil && binary.LittleEndian.Uint16(raw[1+i*2:]) != *want {
			return false
		}
	}
	return p.Value == nil || uint32(raw[8])|binary.LittleEndian.Uint32(raw[9:13])<<8 == *p.Value
}

type StoryOperation struct {
	Index byte   `json:"index"`
	Code  byte   `json:"code"`
	D1    uint16 `json:"d1,omitempty"`
	D2    uint16 `json:"d2,omitempty"`
	D3    uint16 `json:"d3,omitempty"`
	D4    uint16 `json:"d4,omitempty"`
	Value uint32 `json:"value,omitempty"`
	DW2   uint32 `json:"dw2,omitempty"`
}

func (s StoryOperation) Operation() Operation {
	o := Operation{Index: s.Index}
	o.Data[0] = s.Code
	for i, v := range []uint16{s.D1, s.D2, s.D3, s.D4} {
		binary.LittleEndian.PutUint16(o.Data[1+i*2:], v)
	}
	if s.Value != 0 {
		o.Data[8] = byte(s.Value)
		binary.LittleEndian.PutUint32(o.Data[9:13], s.Value>>8)
	}
	binary.LittleEndian.PutUint32(o.Data[13:17], s.DW2)
	return o
}

type StoryEdit struct {
	Kind         string           `json:"kind"` // remove, prepend, replace_first, move_before, copy_speech
	Match        StoryPattern     `json:"match"`
	Before       StoryPattern     `json:"before"`
	SourceBranch byte             `json:"source_branch,omitempty"`
	Operations   []StoryOperation `json:"operations,omitempty"`
}

type StoryRule struct {
	LaterMovie    uint32           `json:"later_movie,omitempty"`
	Destination   StoryDestination `json:"destination"`
	UsePreEvent   bool             `json:"use_pre_event,omitempty"`
	TargetActors  []uint16         `json:"target_actors,omitempty"`
	Cues          []StoryCue       `json:"cues,omitempty"`
	TargetPattern StoryPattern     `json:"target_pattern"`
	ID            string           `json:"id"`
	Kind          string           `json:"kind"`
	Scope         StoryScope       `json:"scope"`
	Gates         []StoryGate      `json:"gates,omitempty"`
	Pattern       StoryPattern     `json:"pattern"`
	// Value is the typed kind's result: branch priority, mode, visibility, or margin.
	Value        int64       `json:"value,omitempty"`
	TargetEvent  uint16      `json:"target_event,omitempty"`
	TargetActor  uint16      `json:"target_actor,omitempty"`
	TargetBranch byte        `json:"target_branch,omitempty"`
	Template     uint32      `json:"template,omitempty"`
	Mark         uint32      `json:"mark,omitempty"`
	Text         string      `json:"text,omitempty"`
	Edits        []StoryEdit `json:"edits,omitempty"`
}

type StoryMark struct {
	PreserveStep bool   `json:"preserve_step,omitempty"`
	ID           uint32 `json:"id"`
	Mode         string `json:"mode"` // complete, advance, ensure_active, set
	Value        int    `json:"value"`
}

type StoryLine struct {
	Talk     uint32 `json:"talk"`
	Portrait byte   `json:"portrait"`
}

type StoryRecruitment struct {
	ID              string      `json:"id"`
	Scope           StoryScope  `json:"scope"`
	Template        uint32      `json:"template"`
	Name            string      `json:"name"`
	CompletedMark   uint32      `json:"completed_mark"`
	Marks           []StoryMark `json:"marks"`
	Lines           []StoryLine `json:"lines"`
	RepeatLines     []StoryLine `json:"repeat_lines"`
	FullPartyLines  []StoryLine `json:"full_party_lines"`
	FullPartyPrefix int         `json:"full_party_prefix"`
	PostureMovie    uint32      `json:"posture_movie"`
	MovieMode       byte        `json:"movie_mode"`
	UnavailableText string      `json:"unavailable_text"`
	JoinedText      string      `json:"joined_text"`
}

type StoryTransformation struct {
	UnavailableText string               `json:"unavailable_text"`
	ID              string               `json:"id"`
	Scope           StoryScope           `json:"scope"`
	Pattern         StoryPattern         `json:"pattern"`
	Gates           []StoryGate          `json:"gates"`
	Marks           []StoryMark          `json:"marks"`
	Body            uint16               `json:"body"`
	Head            uint16               `json:"head"`
	Model           byte                 `json:"model"`
	HideActor       uint16               `json:"hide_actor"`
	Program         []StoryProgramBranch `json:"program"`
}

type StoryProgramBranch struct {
	Index           byte         `json:"index"`
	Condition       StoryPattern `json:"condition"`
	ExtraConditions int          `json:"extra_conditions"`
}

type StoryReward struct {
	ID            string           `json:"id"`
	Scope         StoryScope       `json:"scope"`
	Pattern       StoryPattern     `json:"pattern"`
	Condition     StoryPattern     `json:"condition"`
	ExtraItems    []QuestItem      `json:"extra_items"`
	Marks         []StoryMark      `json:"marks"`
	Tail          []StoryOperation `json:"tail"`
	CompletedMark uint32           `json:"completed_mark"`
	Message       string           `json:"message"`
	FullText      string           `json:"full_text"`
}

type StoryBattleReward struct {
	AllowMissing bool             `json:"allow_missing,omitempty"`
	Quest        uint32           `json:"quest"`
	Monsters     []uint32         `json:"monsters"`
	Aliases      []string         `json:"aliases"`
	Companion    uint32           `json:"companion"`
	Gold         uint32           `json:"gold"`
	Start        bool             `json:"start"`
	Equipment    []StoryEquipment `json:"equipment"`
}

// Cinematic cues describe presentation; session ownership, ACK gates and timers
// remain in Go. Character IDs are injected by the encoder, never stored in data.
type StoryCue struct {
	Kind      string   `json:"kind"`
	Data      []uint32 `json:"data"`
	Broadcast bool     `json:"broadcast,omitempty"`
	Self      bool     `json:"self,omitempty"`
}
type StoryCinematicStep struct {
	Cues []StoryCue `json:"cues"`
}
type StoryCinematic struct {
	AllowMissing   bool                 `json:"allow_missing,omitempty"`
	Role           string               `json:"role"`
	ID             string               `json:"id"`
	Maps           []uint16             `json:"maps"`
	CompletionMark uint32               `json:"completion_mark"`
	Trigger        StoryPattern         `json:"trigger"`
	DestinationMap uint16               `json:"destination_map"`
	DestinationX   uint16               `json:"destination_x"`
	DestinationY   uint16               `json:"destination_y"`
	Steps          []StoryCinematicStep `json:"steps"`
	Finish         []StoryCue           `json:"finish"`
}

//go:embed story_defaults.json
var storyDefaults []byte

// DefaultStoryDefinitions supplies offline import and independent fixture seeds.
// World/server constructors deliberately do not call this function.
func DefaultStoryDefinitions() StoryDefinitions {
	var d StoryDefinitions
	if err := json.Unmarshal(storyDefaults, &d); err != nil {
		panic(err)
	}
	return d
}

func validateStoryStructure(d StoryDefinitions) error {
	kinds := map[string]bool{}
	for _, k := range []string{StoryForwardOnly, StoryPriority, StoryBlock, StoryContinue, StoryClick, StoryClickMark, StoryCandidate, StoryArrival, StoryArrivalCleanup, StoryArrivalRegion, StoryVisibility, StoryFinalVisibility, StoryCondition, StoryPreCondition, StoryPropCondition, StoryEquipmentGate, StoryPreFallback, StoryMovieMode, StoryChoiceMode, StoryActorCapability, StoryOutcome, StoryRepair, StoryRegionMargin, StoryTransition, StoryExchange, StoryRecruitBlock, StorySyncActors, StoryPreserveActor, StoryArrivalPackets, StoryPortalConsumeVehicle} {
		kinds[k] = true
	}
	seen := map[string]bool{}
	identity := func(id string) error {
		if id == "" || seen[id] {
			return fmt.Errorf("duplicate/empty story identity %q", id)
		}
		seen[id] = true
		return nil
	}
	for _, r := range d.Rules {
		if err := identity(r.ID); err != nil {
			return err
		}
		if !kinds[r.Kind] {
			return fmt.Errorf("story %s: unknown kind %q", r.ID, r.Kind)
		}
		for _, g := range r.Gates {
			if err := validateStoryGate(g); err != nil {
				return fmt.Errorf("story %s: %w", r.ID, err)
			}
		}
		for _, e := range r.Edits {
			switch e.Kind {
			case "remove", "prepend", "replace_first", "move_before", "copy_speech":
			default:
				return fmt.Errorf("story %s: unknown edit %q", r.ID, e.Kind)
			}
		}
		if (r.Kind == StoryMovieMode || r.Kind == StoryChoiceMode) && (r.Value < 0 || r.Value > 2) {
			return fmt.Errorf("story %s: invalid mode", r.ID)
		}
	}
	for _, r := range d.Recruitments {
		if err := identity(r.ID); err != nil {
			return err
		}
		if r.Template == 0 || r.CompletedMark == 0 || r.FullPartyPrefix < 0 || r.FullPartyPrefix > len(r.Lines) {
			return fmt.Errorf("invalid recruitment %s", r.ID)
		}
	}
	for _, r := range d.Transformations {
		if err := identity(r.ID); err != nil {
			return err
		}
		if r.Model == 0 || r.Body == 0 {
			return fmt.Errorf("invalid transformation %s", r.ID)
		}
		for _, g := range r.Gates {
			if err := validateStoryGate(g); err != nil {
				return err
			}
		}
	}
	for _, r := range d.Rewards {
		if err := identity(r.ID); err != nil {
			return err
		}
		if r.CompletedMark == 0 || len(r.Tail) != len(r.Marks) {
			return fmt.Errorf("invalid reward %s", r.ID)
		}
	}
	return nil
}

func validateStoryGate(g StoryGate) error {
	switch g.Source {
	case "mark_state", "mark_step", "active_mark_step", "mark_exists", "party", "party_exact", "active_pet_exact", "party_broadcast", "npc_template", "equipment", "inventory", "active_vehicle":
	default:
		return fmt.Errorf("unknown story gate %q", g.Source)
	}
	switch g.Op {
	case "eq", "ne", "lt", "le", "gt", "ge":
	default:
		return fmt.Errorf("unknown story comparison %q", g.Op)
	}
	return nil
}

type StoryEquipment struct {
	Slot   byte   `json:"slot"`
	ItemID uint16 `json:"item_id"`
}

type StoryCompanionEquipment struct {
	AllowMissing bool             `json:"allow_missing,omitempty"`
	Template     uint32           `json:"template"`
	Equipment    []StoryEquipment `json:"equipment"`
}

type StoryDestination struct {
	Map uint16 `json:"map"`
	X   uint16 `json:"x"`
	Y   uint16 `json:"y"`
}

const (
	StoryOutcomeForward = 1
	StoryOutcomeBounded = 2
)
