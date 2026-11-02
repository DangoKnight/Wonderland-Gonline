package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/game"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

const maxMallWireIndex = math.MaxUint8
const dropRewardColumnCount = 5
const maxDropProbabilityPercent = 100

// Imported compatibility tables may contain unavailable content. Administrator
// edits must be usable immediately rather than silently dropping invalid rows.
func validateAdminAsset(asset string, raw []byte, candidate *assets.Catalog) error {
	switch asset {
	case "item_mall.json":
		for _, item := range candidate.Mall {
			if _, ok := candidate.Items[item.ID]; !ok || item.Cost > game.MaxMallPoints || item.CategoryID < 0 || item.CategoryID > maxMallWireIndex || item.Order < 0 || item.Order > maxMallWireIndex || (item.Bonus != 0 && item.Bonus != 1) {
				return fmt.Errorf("invalid mall item %d or native category/order range", item.ID)
			}
		}
	case "starter_items.json":
		for _, grant := range candidate.StarterItems {
			if _, ok := candidate.Items[grant.ID]; !ok {
				return fmt.Errorf("unknown starter item %d", grant.ID)
			}
		}
	case "monster_drops.txt":
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return err
		}
		for _, line := range strings.Split(doc.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "TID:") {
				return errors.New("drop lines must start with TID:")
			}
			columns := strings.Split(line, "|")
			id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(columns[0], "TID:")), 10, 16)
			if err != nil || id == 0 {
				return errors.New("invalid monster ID")
			}
			if _, ok := candidate.NPCs[uint16(id)]; !ok {
				return errors.New("unknown drop monster")
			}
			for _, reward := range columns[1:] {
				fields := strings.Split(strings.TrimSpace(reward), ",")
				if len(fields) != dropRewardColumnCount {
					return errors.New("drop rewards need item,name,min,max,percent")
				}
				item, e1 := strconv.ParseUint(strings.TrimSpace(fields[0]), 10, 16)
				low, e2 := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 8)
				high, e3 := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 8)
				rate, e4 := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
				_, known := candidate.Items[uint16(item)]
				if e1 != nil || e2 != nil || e3 != nil || e4 != nil || !known || low == 0 || low > high || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > maxDropProbabilityPercent {
					return errors.New("invalid drop item, quantity or probability")
				}
			}
		}
	}
	return nil
}

type AssetDocument struct {
	Asset  string `json:"asset"`
	Origin string `json:"origin"`
	Source string `json:"source"`
}
type AssetEdit struct {
	Version string          `json:"version"`
	Value   json.RawMessage `json:"value"`
}

func (s *Server) AssetDocuments(ctx context.Context) ([]AssetDocument, error) {
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return nil, err
	}
	defer assetdb.Close(db)
	out := []AssetDocument{}
	for _, name := range assetsql.DefinitionNames {
		out = append(out, AssetDocument{Asset: name, Origin: "SQL", Source: "assets database"})
	}
	return out, nil
}
func (s *Server) ReadAssetDocument(ctx context.Context, asset string) (AssetEdit, error) {
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return AssetEdit{}, err
	}
	defer assetdb.Close(db)
	var raw []byte
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; raw, err = assetsql.ReadDefinition(tx, asset); return err })
	if err != nil {
		return AssetEdit{}, err
	}
	return AssetEdit{Version: assetdb.DocumentVersion(raw), Value: raw}, nil
}

type AssetRecord struct {
	Collection string `json:"collection"`
	Ordinal    int    `json:"ordinal"` // Numeric game identity, independent of source position.
	AssetEdit
}

func (s *Server) ReadAssetRecords(ctx context.Context, asset string, id int64) ([]AssetRecord, error) {
	if id < 0 || uint64(id) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("invalid asset identity")
	}
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return nil, err
	}
	defer assetdb.Close(db)
	var raw []byte
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		raw, err = assetsql.ReadDefinitionRecord(tx, asset, int(id))
		return err
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []AssetRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []AssetRecord{{Collection: asset, Ordinal: int(id), AssetEdit: AssetEdit{Version: assetdb.DocumentVersion(raw), Value: raw}}}, nil
}

func (s *Server) EditAsset(ctx context.Context, asset string, edit AssetEdit, collection string, ordinal int) error {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	for _, c := range s.friendSessions {
		if !gmIdle(c) {
			return errors.New("finish active interactions before editing assets")
		}
	}
	s.mu.Lock()
	loading := false
	for _, c := range s.sessions {
		if c.info.CharacterID != 0 && s.friendSessions[c.info.CharacterID] != c {
			loading = true
			break
		}
	}
	s.mu.Unlock()
	if loading {
		return errors.New("finish map loading before editing assets")
	}
	if _, err := os.Stat(s.Config.AssetsDatabase); err != nil {
		return err
	}
	db, err := assetdb.Open(s.Config.AssetsDatabase)
	if err != nil {
		return err
	}
	defer assetdb.Close(db)
	var candidate *assets.Catalog
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current []byte
		var err error
		var recordID *int
		if collection == "" {
			current, err = assetsql.ReadDefinition(tx, asset)
		} else {
			if collection != asset {
				return errors.New("record collection does not match dataset")
			}
			recordID = &ordinal
			current, err = assetsql.ReadDefinitionRecord(tx, asset, ordinal)
		}
		if err != nil {
			return err
		}
		if edit.Version == "" || assetdb.DocumentVersion(current) != edit.Version {
			return assetdb.ErrEditConflict
		}
		candidate, err = assetsql.DefinitionCandidate(tx, asset, edit.Value, recordID)
		if err != nil {
			return err
		}
		for id := range s.Assets.Maps {
			if _, ok := candidate.Maps[id]; !ok {
				return fmt.Errorf("cannot remove existing map %d", id)
			}
		}
		return assetsql.SaveEditedCatalog(tx, candidate)
	})
	if err != nil {
		return err
	}
	candidate.AssetsDatabase = s.Config.AssetsDatabase
	*s.Assets = *candidate
	s.alchemyIndex = nil // Native item edits invalidate derived rank/base candidate indexes.
	s.World.ReloadCatalog(s.Assets)
	// Reconnect completes native catalog and map snapshots coherently.
	for _, c := range s.friendSessions {
		if err := s.autosaveSession(ctx, c); err != nil {
			s.Log.Error("asset edit autosave failed", "character", c.character.ID, "error", err)
		}
		c.conn.Close()
	}
	s.Log.Info("administrator asset edit", "asset", asset, "collection", collection, "ordinal", ordinal)
	return nil
}
func (s *Server) EnsureAdminAsset(ctx context.Context, asset string) error {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	if _, err := os.Stat(s.Config.AssetsDatabase); err != nil {
		return err
	}
	db, err := assetdb.Open(s.Config.AssetsDatabase)
	if err != nil {
		return err
	}
	defer assetdb.Close(db)
	_, err = assetsql.ReadDefinition(db.WithContext(ctx), asset)
	return err
}

type AdminOpcode struct {
	Operation world.Op `json:"operation"`
	Meaning   string   `json:"meaning"`
	Talk      string   `json:"talk,omitempty"`
}
type AdminEventBranch struct {
	Index      byte          `json:"index"`
	Condition  world.Cond    `json:"condition"`
	Operations []AdminOpcode `json:"operations"`
}
type AdminActorEvent struct {
	ID       uint16             `json:"id"`
	Name     string             `json:"name"`
	Branches []AdminEventBranch `json:"branches"`
}
type AdminActorReport struct {
	MapID          uint16            `json:"map_id"`
	Actor          assets.MapNPC     `json:"actor"`
	LinkedQuestIDs []uint32          `json:"linked_quest_ids"`
	Events         []AdminActorEvent `json:"events"`
}

func actorReport(catalog *assets.Catalog, mapID, click uint16) (AdminActorReport, error) {
	m, ok := catalog.Maps[mapID]
	if !ok {
		return AdminActorReport{}, errors.New("unknown map")
	}
	var actor assets.MapNPC
	found := false
	for _, n := range m.NPCs {
		if n.ClickID == click {
			actor = n
			found = true
			break
		}
	}
	if !found {
		return AdminActorReport{}, errors.New("unknown actor")
	}
	report := AdminActorReport{MapID: mapID, Actor: actor, LinkedQuestIDs: []uint32{}, Events: []AdminActorEvent{}}
	ids := map[uint16]bool{}
	for _, id := range actor.Events {
		ids[uint16(id)] = true
	}
	if len(ids) == 0 {
		ids[click] = true
	}
	linked := map[uint32]bool{}
	for _, ev := range m.Events {
		if !ids[ev.ClickID] {
			continue
		}
		entry := AdminActorEvent{ID: ev.ClickID, Name: ev.Name, Branches: []AdminEventBranch{}}
		for _, branch := range ev.Branches {
			cond := world.DecodeCond(branch.Condition)
			row := AdminEventBranch{Index: branch.Index, Condition: cond, Operations: []AdminOpcode{}}
			for _, raw := range branch.Operations {
				op := world.DecodeOp(raw)
				meaning := "unresolved"
				talk := ""
				switch op.Code {
				case world.ActionActor:
					meaning = "actor action"
					if op.D2 == world.ActorActionPropState {
						meaning = "prop frame / break animation"
						if cond.Kind == world.ConditionQuest && cond.W1 != 0 {
							linked[uint32(cond.W1)] = true
						}
					}
				case world.ActionPlayer:
					meaning = "player action"
				case world.ActionQuestMark:
					meaning = "quest mark action"
				case world.ActionCompanion:
					meaning = "companion action"
				case world.ActionBattle, world.ActionBattleAlternate:
					meaning = "battle"
				case world.ActionServiceOrTeleport:
					meaning = "NPC service or map transition"
				case world.ActionMovie:
					meaning = "movie"
				case world.ActionMinigame:
					meaning = "minigame"
				case world.ActionLearnSkill:
					meaning = "learn skill"
				case world.ActionTransform:
					meaning = "character transformation"
				case world.ActionMinimapMarker:
					meaning = "minimap marker"
				case world.ActionGather:
					meaning = "gathering"
				case world.ActionEffect:
					meaning = "effect"
				}
				if op.Code == world.ActionActor && op.D2 != world.ActorActionPropState && op.D2 != world.ActorActionAnimation && op.D2 != world.ActorActionAnimationAlternate && op.D2 != world.ActorActionPath && op.D2 != world.ActorActionPose {
					var talkID uint32
					if op.D3 >= world.MinDialogueID && op.D3 <= world.MaxDialogueWordID {
						talkID = uint32(op.D3) | uint32(op.D2)<<16
					} else if op.D2 >= world.MinDialogueID && op.D2 <= world.MaxDialogueWordID {
						talkID = uint32(op.D2)
					}
					if talkID >= world.MinDialogueID {
						meaning = "dialogue"
						talk, _ = catalog.Talks.Resolve(talkID)
					}
				}
				row.Operations = append(row.Operations, AdminOpcode{Operation: op, Meaning: meaning, Talk: talk})
			}
			entry.Branches = append(entry.Branches, row)
		}
		report.Events = append(report.Events, entry)
	}
	for id := range linked {
		report.LinkedQuestIDs = append(report.LinkedQuestIDs, id)
	}
	sort.Slice(report.LinkedQuestIDs, func(i, j int) bool { return report.LinkedQuestIDs[i] < report.LinkedQuestIDs[j] })
	return report, nil
}
func (s *Server) AdminNPCReport(template uint32) []AdminActorReport {
	catalog := s.AssetSnapshot()
	out := []AdminActorReport{}
	maps := make([]int, 0, len(catalog.Maps))
	for id := range catalog.Maps {
		maps = append(maps, int(id))
	}
	sort.Ints(maps)
	for _, id := range maps {
		for _, n := range catalog.Maps[uint16(id)].NPCs {
			if n.Template == template {
				report, err := actorReport(catalog, uint16(id), n.ClickID)
				if err == nil {
					out = append(out, report)
				}
			}
		}
	}
	return out
}
func (s *Server) AdminMapActors(mapID uint16) ([]AdminActorReport, error) {
	catalog := s.AssetSnapshot()
	m, ok := catalog.Maps[mapID]
	if !ok {
		return nil, errors.New("unknown map")
	}
	out := []AdminActorReport{}
	for _, n := range m.NPCs {
		report, err := actorReport(catalog, mapID, n.ClickID)
		if err != nil {
			return nil, err
		}
		out = append(out, report)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Actor.ClickID < out[j].Actor.ClickID })
	return out, nil
}

type AdminActorEdit struct {
	Version       string `json:"version"`
	ClickID       uint16 `json:"click_id"`
	Action        string `json:"action"`
	Scope         string `json:"scope"`
	LinkedQuestID uint32 `json:"linked_quest_id"`
}

// EditAdminActor ties durable linked marks to an optimistic owned-character edit.
// Visibility/frame publication is visit-local; it never edits other players' quests.
func (s *Server) EditAdminActor(ctx context.Context, id uint32, edit AdminActorEdit) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.adminCharacterLoading(id) {
		return ErrAdminPlayerUnavailable
	}
	row, err := s.Store.AdminCharacter(ctx, id)
	if err != nil {
		return err
	}
	if edit.Version == "" || edit.Version != row.Version {
		return store.ErrAdminConflict
	}
	if edit.Scope != "session" && edit.Scope != "map" {
		return errors.New("actor scope must be session or map")
	}
	if edit.Action != "show" && edit.Action != "hide" && edit.Action != "open" && edit.Action != "close" {
		return errors.New("invalid actor action")
	}
	report, err := actorReport(s.Assets, row.State.Map, edit.ClickID)
	if err != nil {
		return err
	}
	c := s.friendSessions[id]
	if c != nil && (!gmIdle(c) || c.stall != nil || c.openTent != nil || c.tentOwner != 0) {
		return ErrAdminPlayerUnavailable
	}
	quest := edit.LinkedQuestID
	if quest != 0 {
		found := false
		for _, qid := range report.LinkedQuestIDs {
			if qid == quest {
				found = true
			}
		}
		if !found {
			return errors.New("quest is not linked to this prop")
		}
	}
	if quest == 0 && (edit.Action == "open" || edit.Action == "close") {
		if len(report.LinkedQuestIDs) > 1 {
			return errors.New("select one of the linked quest IDs")
		}
		if len(report.LinkedQuestIDs) == 1 {
			quest = report.LinkedQuestIDs[0]
		}
	}
	if quest != 0 {
		if edit.Action != "open" && edit.Action != "close" {
			return errors.New("linked quest edits require open or close")
		}
		next := row.State.Clone()
		if edit.Action == "open" {
			if next.Quests == nil {
				next.Quests = map[uint32]game.Quest{}
			}
			q := next.Quests[quest]
			q.ID = quest
			q.State = game.Completed
			q.Step = max(1, q.Step)
			if q.StartedAt.IsZero() {
				q.StartedAt = time.Now().UTC()
			}
			now := time.Now().UTC()
			q.CompletedAt = &now
			next.Quests[quest] = q
		} else {
			delete(next.Quests, quest)
		}
		if err = s.Store.ReplaceAdminCharacter(ctx, id, edit.Version, next); err != nil {
			return err
		}
		if c != nil {
			s.adoptSavedCharacter(c, next)
			if err := s.sendAll(c, s.World.Journal(c.character, c.view)); err != nil {
				c.conn.Close()
			}
		}
	} else if c == nil {
		return ErrAdminPlayerUnavailable
	}
	if c == nil {
		return nil
	}
	targets := []*Session{c}
	if edit.Scope == "map" {
		targets = append(targets, s.peers(c)...)
	}
	for _, target := range targets {
		if target.view == nil {
			continue
		}
		var packet []byte
		switch edit.Action {
		case "hide", "show":
			if target.view.AdminActors == nil {
				target.view.AdminActors = map[uint16]bool{}
			}
			shown := edit.Action == "show"
			target.view.AdminActors[edit.ClickID] = shown
			if shown {
				packet = s.World.ShowActor(target.view, target.character.Map, edit.ClickID)
			} else {
				packet = s.World.HideActor(target.view, target.character.Map, edit.ClickID)
			}
		case "open", "close":
			frame := byte(propIntactFrame)
			if edit.Action == "open" {
				frame = propBrokenFrame
			}
			target.view.Props[edit.ClickID] = int32(frame)
			packet = serverwire.PropFrame(edit.ClickID, frame)
		}
		s.sendOrClose(target, packet)
	}
	return nil
}
