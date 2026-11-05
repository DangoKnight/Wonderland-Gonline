package server

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestInstalledHoneycombUsesModularStoryPolicies(t *testing.T) {
	const kelanMap, honeyEvent, honeyActor = 12000, 25, 17
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		configured := *catalog
		configured.Story.Rules = nil
		for _, rule := range catalog.Story.Rules {
			if rule.ID == "honeycomb_start_bee_battle" {
				if !enabled {
					continue
				}
				// Identity is an administrator label, never a runtime switch.
				rule.ID = "operator_authored_pickup_continuation"
			}
			configured.Story.Rules = append(configured.Story.Rules, rule)
		}
		s, players, _ := worldFixture(t)
		c := players[0]
		s.Assets, s.World = &configured, world.New(&configured)
		c.character.Map, c.character.X, c.character.Y = kelanMap, 195, 1568
		c.character.Bag, c.character.Quests = game.Inventory{}, map[uint32]game.Quest{}
		saveBreillatFixture(t, s, c)
		event, _ := s.World.Event(kelanMap, honeyEvent)
		before, _ := json.Marshal(event)
		branch := s.World.FindBranch(c.character, c.view, kelanMap, event, 0, 0, 0, -1)
		if branch < 0 {
			t.Fatal("missing native pickup branch")
		}
		if err := s.startEvent(context.Background(), c, honeyActor, event, branch, true); err != nil {
			t.Fatal(err)
		}
		for step := 0; step < 10 && c.event != nil && c.event.onComplete != nil && c.battle == nil; step++ {
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
		}
		if (c.battle != nil) != enabled {
			t.Fatal("battle ignored authored continuation policy", enabled)
		}
		after, _ := json.Marshal(event)
		if !bytes.Equal(before, after) {
			t.Fatal("native event data mutated")
		}
		for _, rule := range configured.Story.Rules {
			if rule.Kind == assets.StoryRepair && rule.ID == "honeycomb_battle_completion" {
				t.Fatal("obsolete quest repair enabled")
			}
		}
	}
}

func TestInstalledHoneycombBees(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []uint16{1, 2} {
		s, players, _ := worldFixture(t)
		c := players[0]
		s.Assets, s.World = catalog, world.New(catalog)
		c.character.Map, c.character.X, c.character.Y = 12000, 195, 1568
		c.character.Bag = game.Inventory{}
		c.character.Quests = map[uint32]game.Quest{13022: {ID: 13022, State: game.InProgress, Step: 1}}
		saveBreillatFixture(t, s, c)
		for _, click := range []uint16{18, 19} {
			if !s.World.Visible(c.character, 12000, click) {
				t.Fatal("fresh bees hidden", click)
			}
			n, _ := s.World.NPC(12000, click)
			if s.World.Wild(12000, n) {
				t.Fatal("quest bee became ambient attacker")
			}
		}
		now := time.Now()
		s.World.AdvanceActors(12000, now, nil, func(int) int { return 0 })
		moved := map[uint16]bool{}
		rng := rand.New(rand.NewPCG(1, 2))
		for tick := 1; tick <= 30; tick++ {
			for _, m := range s.World.AdvanceActors(12000, now.Add(time.Duration(tick)*time.Second), nil, rng.IntN) {
				moved[m.Click] = true
			}
		}
		if !moved[18] || !moved[19] {
			t.Fatal("bees do not roam", moved)
		}
		ev, _ := s.World.Event(12000, 25)
		branch := s.World.FindBranch(c.character, c.view, 12000, ev, 0, 0, 0, -1)
		if branch < 0 || ev.Branches[branch].Index != 1 {
			t.Fatal("pickup branch", branch)
		}
		if err := s.startEvent(context.Background(), c, 17, ev, branch, true); err != nil {
			t.Fatal(err)
		}
		// The item grant continues into the warning, then the authored battle.
		for i := 0; i < 10 && c.battle == nil && c.event != nil; i++ {
			if c.event.onComplete == nil {
				t.Fatal("honeycomb chain stalled")
			}
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
		}
		if c.battle == nil {
			t.Fatal("pickup did not trigger bees")
		}
		if got := s.World.Formation(12000, 1); len(got) != 2 || got[0] != 17064 || got[1] != 17065 {
			t.Fatal(got)
		}
		origin := c.battle.event.branch
		s.abandonBattle(c)
		outcome := s.World.OutcomeBranch(c.character, c.view, 12000, ev, 1, result, origin)
		if outcome < 0 {
			t.Fatal("missing outcome", result)
		}
		if err := s.startEvent(context.Background(), c, 17, ev, outcome, true); err != nil {
			t.Fatal(err)
		}
		if _, exists := c.character.Quests[13023]; exists {
			t.Fatal("bee outcome advanced hand-in mark", result)
		}
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		var restored *game.Character
		for i := range stored {
			if stored[i].ID == c.character.ID {
				restored = &stored[i]
			}
		}
		if restored == nil || restored.Quests[13023].State != game.NotStarted {
			t.Fatal("hand-in state changed before reward")
		}
		for _, click := range []uint16{17, 18, 19} {
			if s.World.VisibleIn(restored, world.NewView(), 12000, click) {
				t.Fatal("bee returns on new visit", result, click)
			}
			if !s.World.Visible(&game.Character{}, 12000, click) {
				t.Fatal("other player's bee hidden")
			}
		}
		// Native pre-events reconstruct hiding while carrying honey, without
		// completing 13023. The patient must still be down on a fresh visit.
		c.character.Map = 12004
		c.view = world.NewView()
		down := false
		posePackets := s.World.Sync(c.character, c.view, true)
		for _, packet := range posePackets {
			down = down || len(packet) == 6 && bytes.Equal(packet[:5], []byte{22, 9, 2, 0, 26})
		}
		if !down {
			t.Fatal("patient stood up before hand-in", posePackets)
		}
		handIn, _ := s.World.Event(12004, 2)
		branch = s.World.FindBranch(c.character, c.view, 12004, handIn, 0, 0, 0, -1)
		if branch < 0 || handIn.Branches[branch].Index != 6 {
			t.Fatal("honey hand-in blocked after bee battle", branch)
		}
		if err := s.startEvent(context.Background(), c, 1, handIn, branch, true); err != nil {
			t.Fatal(err)
		}
		finishBreillatDialogue(t, s, c)
		if c.event != nil || c.character.Quests[13022].State != game.Completed || c.character.Quests[13023].Step != 1 {
			t.Fatal("hand-in did not finish native progression", c.character.Quests)
		}
		counts := map[uint16]int{}
		for _, item := range c.character.Bag {
			counts[item.ID] += int(item.Count)
		}
		if counts[30034] != 0 || counts[23108] != 1 {
			t.Fatal("hand-in did not exchange honey for reward")
		}
		stored, err = s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		for i := range stored {
			if stored[i].ID == c.character.ID {
				restored = &stored[i]
			}
		}
		if restored.Quests[13022].State != game.Completed || restored.Quests[13023].Step != 1 {
			t.Fatal("hand-in progress not durable")
		}
		if next := s.World.FindBranch(restored, world.NewView(), 12004, handIn, 0, 0, 0, -1); next >= 0 && handIn.Branches[next].Index == 6 {
			t.Fatal("completed hand-in can grant a second reward")
		}
		for _, packet := range s.World.Sync(c.character, world.NewView(), true) {
			if len(packet) == 6 && bytes.Equal(packet[:5], []byte{22, 9, 2, 0, 26}) {
				t.Fatal("patient still down after hand-in")
			}
		}
		for _, click := range []uint16{17, 18, 19} {
			if s.World.VisibleIn(c.character, world.NewView(), 12000, click) {
				t.Fatal("honey actors returned after hand-in", click)
			}
		}
	}
}
