// Package livetest prepares isolated, persistent fixtures for manual aLogin tests.
package livetest

import (
	"fmt"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const (
	fixtureBody          = 3
	fixtureHead          = 0
	fixtureSlot          = 1
	fixtureConstitution  = 5
	raftWidth            = 4
	raftHeight           = 3
	starterBeachMap      = 11016
	starterBeachX        = 1181
	starterBeachY        = 243
	beachMap             = 10035
	robinsonActor        = 1
	raftItem             = 48016
	robinsonRaftMark     = 12046
	robinsonDialogueMark = 12047
	robinsonJoinedMark   = 15283
	robinsonPet          = 12032
	beginnerGrade        = 1
)

type Scenario struct {
	ID, Description string
	Steps           []string
}

// Scenarios is the manual acceptance registry. Preparation never simulates a
// successful client interaction or marks the checklist as passed.
func Scenarios() []Scenario {
	return []Scenario{
		{"native-commands", "Native synchronization, appearance, gestures, mall and reconnect", []string{
			"Log in with both accounts in separate legacy-client instances. Select the prepared characters and wait for the map to load.",
			"Open character/inventory panels. Confirm model, element, equipment and HP/SP are displayed correctly; record a screenshot.",
			"Move with ordinary walking and any native waypoint controls available. Record AC6/AC7 requests and replies; confirm the observer sees the correct coordinates once. Only claim acceptance for the AC7 layout actually captured; the inferred map/X/Y adapter remains NOT TESTED unless emitted by aLogin.",
			"Open the emote/expression panel and select one expression twice, allowing it to finish between clicks. Both must appear above the tester on the observer client. Separately sit/stand or use another pose, stop it, and walk again; the observer must see the pose change and movement resume. Native expressions use AC32:1; poses use AC32:2 and stop AC32:3. AC18 is NOT TESTED if no request appears.",
			"Click the Item Mall button. Check that its item list and point balance load; close and reopen it once. Do not buy anything. Compare inventory and balance before/after; both must be unchanged. Close the mall and walk. Mark this UI check PASS if it works; the developer will inspect AC13/21/23 aliases in the log.",
			"Keep the observer logged in. Log out the tester, log back in with the same account, and select LiveTester. The observer must see exactly one tester at the saved location. Then keep the tester online and reconnect LiveObserver; both characters must still see each other once. Save screenshots if a character is missing or duplicated.",
			"SKIP step 7 and mark NOT TESTED. This is internal packet-log review for the developer; you have no client action to perform.",
			"Walk next to a recognizable landmark. Take a screenshot showing your position and Inventory. Use the client logout/character-selection flow, log back in with the same tester account, and choose LiveTester. You should return to the same position with the same appearance and HP/SP. Compare with the screenshot and mark PASS or FAIL.",
			"Open Social, then its Setup/settings form. Enter a short custom title/nickname, select blood type and enter a valid birthday; save. Reopen and reconnect to verify the fields persist. Try saving with blood type missing: the warning must not disconnect you or replace the previous blood type/birthday. The Social custom title is distinct from awarded titles, reborn jobs and bath flows; those unavailable flows remain NOT TESTED.",
		}},
		{"carnie-return", "Carnie scripted exit returns to the visit origin", []string{
			"The tester starts on Starter Beach (11016), where native Carnie teleport is available. Record the initial coordinates from these instructions. Enter /carnie in local chat, or use the native Carnie teleport menu.",
			"After Carnie loads, walk away from the arrival point and through its exit. Confirm the character returns to the original map and coordinates, rather than Underground Maze.",
			"Walk on the returned map, then log out and reconnect. Confirm movement was saved and the logs contain no autosave conflict. Repeat the Carnie round trip once.",
		}},
		{"fishing", "Fishing without a learned skill", fishingSteps(false, false)},
		{"fishing-skilled", "Fishing with a learned grade-one skill", fishingSteps(true, false)},
		{"fishing-full-bag", "Full-bag fishing preserves proficiency without item delivery", fishingSteps(true, true)},
		{"fishing-advancement", "Fishing one catch before grade advancement and reconnect", append(fishingSteps(true, false), "The tester starts one catch below the grade-one threshold. The first due catch must advance to grade two with zero per-grade EXP; capture AC8:1 stats 110/111, reconnect, then check that another catch increments progress without a second premature grade change.")},
		{"inventory-raft", "Raft footprint, movement and reconnect", []string{
			"Open Inventory. The raft is the anchor in slot 1; confirm it occupies its full 4 by 3 footprint without duplicate icons.",
			"Move the raft to an empty valid position, then attempt a position extending past the right or bottom edge. The invalid move must fail without losing the raft.",
			"Disconnect and reconnect. Confirm exactly one raft remains and its footprint/anchor position are restored.",
		}},
		{"raft-no-space", "Reject raft pickup in a fragmented inventory", []string{
			"Open Inventory. Alternate cells contain fishing rods; many cells are empty, but no 4 by 3 rectangle is available.",
			"Interact with the raft chest on the beach (actor 7). Acquisition must fail; no raft, recruitment progress or existing items should be lost.",
			"Free a 4 by 3 rectangle by removing the obstructing rods, retry the chest, and complete its dialogue. Confirm exactly one raft is delivered and Robinson recruitment continues.",
			"Reconnect and verify items and recruitment remain saved. Record any difference between the displayed inventory and server acceptance.",
		}},
		{"robinson-recovery", "Recover Robinson after dialogue was saved but recruitment was interrupted", []string{
			"The character starts near Robinson with mark 12046 completed, mark 12047 at step 1, no raft and no recruited Robinson.",
			"Talk to Robinson (actor 1) and complete any dialogue. Confirm Robinson joins the companion roster and disappears from the map for this character.",
			"Reconnect. Confirm one Robinson, recruitment mark 15283 at step 1, and no newly granted raft. Repeated interactions must not duplicate the companion.",
		}},
	}
}

func fishingSteps(skilled, full bool) []string {
	steps := []string{
		"The character is on walkable shoreline selected from the SQL terrain. Leave the observer ungrouped; do not mount or enter another interaction.",
		"Double-click the fishing rod in Inventory. Confirm casting begins without errors; the request should be AC23:53. Wait for at least two configured catch intervals without moving.",
		"Check each delivered catch's item flight toward the character, acquisition line in chat and private Fishing: caught notification. The observer must receive no catch notification or inventory reward. Check that the observer sees the fishing rod, including after leaving and returning to the map. Check catch delivery and logs. Record catch timestamps and compare the spacing with the configured interval (allow normal tick/network delay). Stop casting with the native UI (AC23:54), then wait another interval: no further catch should arrive and the observer's rod should disappear.",
		"Start again and move; casting must stop. Repeat using logout/reconnect: offline time must not grant catches or reset a future cooldown.",
	}
	if !skilled {
		steps = append(steps, "Confirm catches work without a learned fishing skill and do not auto-teach the skill; reconnect and check that the skill remains absent.")
	}
	if skilled {
		steps = append(steps, "Check the fishing EXP overlay after each catch, grade advancement and persistence after reconnect. One successful due catch gives one proficiency point; reaching the configured threshold advances grade.")
	}
	if full {
		steps = append(steps, "The bag contains only full stacks of rods and starts full. Due catches must retain proficiency but add no item. Free a cell and recast: a later catch should be delivered normally.")
	}
	return steps
}

func Find(id string) (Scenario, error) {
	for _, s := range Scenarios() {
		if s.ID == id {
			return s, nil
		}
	}
	return Scenario{}, fmt.Errorf("unknown scenario %q; use -list", id)
}

func shoreline(a *assets.Catalog) (game.Location, error) {
	if !a.Fishing.Enabled {
		return game.Location{}, fmt.Errorf("fishing is disabled in assets.db")
	}
	for _, id := range a.Fishing.Maps {
		t, ok := a.Terrains[id]
		if !ok {
			continue
		}
		if _, ok := a.Maps[id]; !ok {
			continue
		}
		for x := assets.TerrainCellSize; x < int(t.Width); x += assets.TerrainCellSize {
			for y := assets.TerrainCellSize; y < int(t.Height); y += assets.TerrainCellSize {
				if t.FishingShore(uint16(x), uint16(y)) {
					return game.Location{Map: id, X: uint16(x), Y: uint16(y)}, nil
				}
			}
		}
	}
	return game.Location{}, fmt.Errorf("no configured walkable fishing shoreline in assets.db")
}

// Use the authored public teleport return point on Starter Beach. Robinson's
// rescue island is unsuitable for native-client Carnie travel acceptance.
func starterBeachLocation(a *assets.Catalog) (game.Location, error) {
	if _, ok := a.Maps[starterBeachMap]; !ok {
		return game.Location{}, fmt.Errorf("Starter Beach map missing")
	}
	terrain, ok := a.Terrains[starterBeachMap]
	if !ok || !terrain.Walkable(starterBeachX, starterBeachY) {
		return game.Location{}, fmt.Errorf("Starter Beach test spawn is not walkable")
	}
	return game.Location{Map: starterBeachMap, X: starterBeachX, Y: starterBeachY}, nil
}

func nearRobinson(a *assets.Catalog) (game.Location, error) {
	m, ok := a.Maps[beachMap]
	if !ok {
		return game.Location{}, fmt.Errorf("Robinson beach map missing")
	}
	terrain, ok := a.Terrains[beachMap]
	if !ok {
		return game.Location{}, fmt.Errorf("Robinson terrain missing")
	}
	for _, npc := range m.NPCs {
		if npc.ClickID != robinsonActor {
			continue
		}
		for dx := -assets.TerrainCellSize; dx <= assets.TerrainCellSize; dx++ {
			for dy := -assets.TerrainCellSize; dy <= assets.TerrainCellSize; dy++ {
				x, y := int(npc.X)+dx, int(npc.Y)+dy
				if terrain.Walkable(x, y) {
					return game.Location{Map: beachMap, X: uint16(x), Y: uint16(y)}, nil
				}
			}
		}
	}
	return game.Location{}, fmt.Errorf("no walkable position near Robinson")
}

// Character builds through normal creation formulas, then applies only the
// scenario's prerequisites. Content and terrain always come from the SQL catalog.
func Character(a *assets.Catalog, scenario string, id uint32, name string, observer bool) (game.Character, error) {
	if _, err := Find(scenario); err != nil {
		return game.Character{}, err
	}
	c, err := game.NewCharacter(id, fixtureSlot, name, game.Appearance{Body: fixtureBody, Head: fixtureHead, Element: game.Water, Base: game.Attributes{Constitution: fixtureConstitution}}, nil, a.Items, time.Now())
	if err != nil {
		return c, err
	}
	var location game.Location
	switch scenario {
	case "carnie-return", "native-commands":
		location, err = starterBeachLocation(a)
	case "fishing", "fishing-skilled", "fishing-full-bag", "fishing-advancement":
		location, err = shoreline(a)
	default:
		location, err = nearRobinson(a)
	}
	if err != nil {
		return c, err
	}
	c.Map, c.X, c.Y = location.Map, location.X, location.Y
	c.Quests = map[uint32]game.Quest{}
	if observer {
		return c, c.Validate()
	}
	switch scenario {
	case "fishing", "fishing-skilled", "fishing-full-bag", "fishing-advancement", "raft-no-space":
		if len(a.Fishing.Rods) == 0 {
			return c, fmt.Errorf("no configured fishing rod")
		}
		rod := a.Fishing.Rods[0].ItemID
		def, ok := a.Items[rod]
		if !ok {
			return c, fmt.Errorf("rod %d missing", rod)
		}
		if err := c.Bag.Add(game.Item{ID: rod}, 1, def.StackLimit(), a.Items); err != nil {
			return c, err
		}
		if scenario == "fishing-full-bag" || scenario == "raft-no-space" {
			w, h := def.Footprint()
			if w != 1 || h != 1 {
				return c, fmt.Errorf("full/fragmented fixture requires a one-cell rod")
			}
			for i := range c.Bag {
				if scenario == "fishing-full-bag" || i%2 == 0 {
					c.Bag[i] = game.Item{ID: rod, Count: def.StackLimit()}
				}
			}
		}
		if scenario == "fishing-skilled" || scenario == "fishing-full-bag" || scenario == "fishing-advancement" {
			if len(a.Fishing.Skills) == 0 {
				return c, fmt.Errorf("no fishing skill configured")
			}
			skill := a.Fishing.Skills[0]
			if _, ok := a.Skills[skill]; !ok {
				return c, fmt.Errorf("fishing skill %d missing", skill)
			}
			exp := uint32(0)
			if scenario == "fishing-advancement" {
				if len(a.Fishing.CatchRequirements) == 0 || a.Fishing.CatchRequirements[0] == 0 {
					return c, fmt.Errorf("grade-one fishing advancement requirement missing")
				}
				exp = a.Fishing.CatchRequirements[0] - 1
			}
			c.Skills = append(c.Skills, game.LearnedSkill{ID: skill, Grade: beginnerGrade, EXP: exp})
		}
	case "inventory-raft":
		def, ok := a.Items[raftItem]
		if !ok {
			return c, fmt.Errorf("raft item missing")
		}
		w, h := def.Footprint()
		if w != raftWidth || h != raftHeight {
			return c, fmt.Errorf("raft footprint is %dx%d; upgrade assets schema/data first", w, h)
		}
		if err := c.Bag.Add(game.Item{ID: raftItem}, 1, def.StackLimit(), a.Items); err != nil {
			return c, err
		}
	case "robinson-recovery":
		if _, ok := a.NPCs[robinsonPet]; !ok {
			return c, fmt.Errorf("Robinson companion template missing")
		}
		c.Quests[robinsonRaftMark] = game.Quest{ID: robinsonRaftMark, State: game.Completed, Step: 1}
		c.Quests[robinsonDialogueMark] = game.Quest{ID: robinsonDialogueMark, State: game.InProgress, Step: 1}
	}
	if _, err := c.Bag.Occupancy(a.Items); err != nil {
		return c, err
	}
	return c, c.Validate()
}
