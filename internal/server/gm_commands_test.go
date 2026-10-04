package server

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestGMCommandCoverage(t *testing.T) {
	// Independent list from AC02's switch and advertised help. Public commands
	// are intentionally available without GM rights.
	expected := strings.Fields("allskills amity b battle bring broadcast buy carnie clearskills cmd cmds dismount droprate exp fight fixall full ghost god godmode gold goto heal help hide hp im info invis invisible item jail kick kickall killall killmonsters level lvl mallpoints maxskills money mute notice online pet petamity petrebirth points points_im rebirth reload repair resetskills resetstats restat shutdown skill sp stat statpoint statpoints stats summon summonall tent town tp unhide unjail unmute unride warp who whois winbattle petlvl petexp clearinv")
	implemented := map[string]bool{}
	for name := range gmCommandRegistry {
		implemented[name] = true
	}
	for _, name := range existingGMCommands {
		implemented[name] = true
	}
	for _, name := range strings.Fields("help cmd cmds unride dismount carnie") {
		implemented[name] = true
	}
	for _, name := range expected {
		if !implemented[name] {
			t.Errorf("Missing GM command %q", name)
		}
	}
}

func TestGMNewCommandsRequirePrivilegesAndIdleOwnership(t *testing.T) {
	s, p, w := chatFixture(t)
	c := p[0]
	before := c.character.Clone()
	for _, spec := range gmCommandDefinitions {
		say(t, s, c, "/"+spec.aliases[0])
		if !reflect.DeepEqual(*c.character, before) || w[0].Len() != 0 {
			t.Fatalf("non-GM command %s", spec.aliases[0])
		}
	}
	c.gmLevel.Store(1)
	c.event = &eventSession{}
	for _, spec := range gmCommandDefinitions {
		if spec.idle {
			say(t, s, c, ":"+spec.aliases[0])
			packets := w[0].packets(t)
			if len(packets) != 1 || !bytes.Contains(packets[0], []byte("Finish active interactions")) || !reflect.DeepEqual(*c.character, before) {
				t.Fatalf("busy command %s", spec.aliases[0])
			}
		}
	}
	c.event = nil
	s.SetGMLevel(c.account.ID, 0)
	say(t, s, c, "/god")
	if !reflect.DeepEqual(*c.character, before) {
		t.Fatal("revoked GM changed state")
	}
}

func TestGMAllSkillsAndGodPersistence(t *testing.T) {
	s, p, w := chatFixture(t)
	c := p[0]
	c.gmLevel.Store(1)
	ids := c.character.GMElementSkillIDs()
	for i, id := range ids {
		if i == 0 {
			id = game.StarterStuntClientID
		}
		s.Assets.Skills[id] = assets.Skill{TableOrder: uint16(i + 1)}
	}
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 12000, Grade: 8, EXP: 97})
	s.Assets.Skills[12000] = assets.Skill{TableOrder: 100}
	say(t, s, c, "/maxskills 255")
	for _, id := range ids {
		found := false
		for _, skill := range c.character.Skills {
			if skill.ID == id {
				found = skill.Grade == 10 && skill.EXP == 0
			}
		}
		if !found {
			t.Fatalf("missing skill %d", id)
		}
	}
	if c.character.Skills[len(c.character.Skills)-1].ID == 0 {
		t.Fatal("invalid skill")
	}
	preserved := false
	for _, sk := range c.character.Skills {
		if sk.ID == 12000 && sk.EXP == 97 && sk.Grade == 8 {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("quest skill overwritten")
	}
	packets := w[0].packets(t)
	if len(packets) != 3 || !bytes.HasPrefix(packets[0], []byte{5, 3}) || !bytes.Equal(packets[1], []byte{5, 4}) {
		t.Fatal("skill snapshot", packets)
	}
	assertProgressSaved(t, s, c)
	before := c.character.Clone()
	delete(s.Assets.Skills, ids[1])
	say(t, s, c, "/allskills 1")
	if !reflect.DeepEqual(*c.character, before) {
		t.Fatal("partial skill grant on catalog failure")
	}
	s.Assets.Skills[ids[1]] = assets.Skill{TableOrder: 2}
	say(t, s, c, "/godmode")
	if c.character.Base != (game.Attributes{Strength: 999, Constitution: 999, Intelligence: 999, Wisdom: 999, Agility: 999}) || c.character.HP != c.character.MaxHP || c.character.SP != c.character.MaxSP {
		t.Fatal("god attributes/vitals")
	}
	assertProgressSaved(t, s, c)
}

func TestGMPetCommandsUseSelectedPetAndNativeSlot(t *testing.T) {
	s, c, w := rebirthFixture(t)
	s.Assets.NPCs[12178] = assets.NPC{ID: 12178, Name: "Robinson", Type: 4}
	c.gmLevel.Store(1)
	for _, wire := range w {
		wire.Reset()
	}
	original := c.character.Clone()
	say(t, s, c, "/petamity 7")
	packets := w[0].packets(t)
	if !contains(packets, []byte{8, 2, 4, 2, 0, 64, 1, 7, 0, 0, 0, 0, 0, 0, 0}) || c.character.Pets[1].Amity != 7 || !reflect.DeepEqual(c.character.Pets[0], original.Pets[0]) {
		t.Fatal("wrong amity pet/slot", packets)
	}
	say(t, s, c, "/petrebirth")
	pet := c.character.Pets[1]
	if !pet.Reborn || pet.Level != 1 || pet.Exp != 0 || pet.StatPoints != 57 || pet.Amity != 100 {
		t.Fatal("GM rebirth", pet)
	}
	if !contains(w[0].packets(t), []byte{69, 1, 2, 1}) {
		t.Fatal("missing native rebirth result")
	}
	before := c.character.Clone()
	say(t, s, c, "/rebirth")
	if !reflect.DeepEqual(*c.character, before) {
		t.Fatal("repeat minted rebirth points")
	}
	say(t, s, c, "/petlevel 5")
	if c.character.Pets[1].Level != 5 || c.character.Pets[1].StatPoints != 69 {
		t.Fatal("pet level points")
	}
	say(t, s, c, "/petexp 100")
	if c.character.Pets[1].Exp == 0 && c.character.Pets[1].Level == 5 {
		t.Fatal("pet EXP unchanged")
	}
	say(t, s, c, "/god")
	pet = c.character.Pets[1]
	if pet.Base.Strength != 999 || pet.HP != pet.MaxHP || pet.SP != pet.MaxSP {
		t.Fatal("god pet")
	}
	normalized := pet
	normalized.Normalize(s.Assets.Items, false)
	if normalized.HP != pet.HP || normalized.MaxHP != pet.MaxHP {
		t.Fatal("god pet differs from reconnect normalization")
	}
	assertProgressSaved(t, s, c)
}

func TestGMRecruitPetPreservesProgressAndRestoresReserve(t *testing.T) {
	s, c, w := rebirthFixture(t)
	s.Assets.NPCs[12178] = assets.NPC{ID: 12178, Name: "Robinson", Type: 4}
	c.gmLevel.Store(1)
	before := c.character.Pets[1]
	say(t, s, c, "/pet 12178 New Robinson")
	pet := c.character.Pets[1]
	if len(c.character.Pets) != 2 || pet.Name != "New Robinson" || pet.Exp != before.Exp || pet.Level != before.Level || c.character.ActivePet != 12178 {
		t.Fatal("duplicate recruitment reset progress")
	}
	next := c.character.Clone()
	reserve := next.Pets[0]
	next.ReservePets = append(next.ReservePets, reserve)
	next.Pets = next.Pets[1:]
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	say(t, s, c, "/pet 14156")
	if len(c.character.ReservePets) != 0 || len(c.character.Pets) != 2 || c.character.ActivePet != 14156 {
		t.Fatal("reserve not restored")
	}
	for _, wire := range w {
		wire.Reset()
	}
	beforeChar := c.character.Clone()
	say(t, s, c, "/pet 65535")
	if !reflect.DeepEqual(*c.character, beforeChar) {
		t.Fatal("unknown pet changed state")
	}
	assertProgressSaved(t, s, c)
}

func TestGMInventoryAndBuyUseNormalCheckout(t *testing.T) {
	s, c, w, _ := mallFixture(t)
	c.gmLevel.Store(1)
	s.Assets.Mall[0].Name = "Healing Potion"
	say(t, s, c, "/buy Healing Potion 2")
	balance, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balance.Points != 80 {
		t.Fatal("mall debit", balance, err)
	}
	count := 0
	for _, item := range c.character.Bag {
		if item.ID == 32176 {
			count += int(item.Count)
		}
	}
	if count != 10 {
		t.Fatal("mall grant", count)
	}
	say(t, s, c, "/buy 32176 255")
	balance, _ = s.Store.MallBalances(context.Background(), c.account.ID)
	if balance.Points != 80 {
		t.Fatal("failed purchase debited")
	}
	s.Assets.Items[34001] = game.ItemDefinition{ID: 34001}
	say(t, s, c, "/tent")
	say(t, s, c, "/tent")
	tents := 0
	for _, item := range c.character.Bag {
		if item.ID == 34001 {
			tents += int(item.Count)
		}
	}
	if tents != 1 {
		t.Fatal("duplicate tent", tents)
	}
	equipment := c.character.Equipment
	w.Reset()
	say(t, s, c, "/clearinv")
	if c.character.Bag != (game.Inventory{}) || c.character.Equipment != equipment {
		t.Fatal("clearinv scope")
	}
	removals := 0
	for _, packet := range w.packets(t) {
		if bytes.HasPrefix(packet, []byte{23, 9}) {
			removals++
			if len(packet) != 4 || packet[3] == 0 {
				t.Fatal("bad bag removal", packet)
			}
		}
	}
	if removals < 2 {
		t.Fatal("missing additive bag removal", removals)
	}
	assertProgressSaved(t, s, c)
}

func TestGMMutePersistsBlocksChannelsAndExpires(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	say(t, s, p[0], "/mute Bobby 1")
	if !time.Now().Before(p[1].character.MutedUntil) {
		t.Fatal("mute not applied")
	}
	assertProgressSaved(t, s, p[1])
	for _, wire := range w {
		wire.Reset()
	}
	for _, sub := range []byte{1, 2, 3, 5, 6} {
		packet := append([]byte{2, sub}, []byte("hello")...)
		if err := s.worldCommand(context.Background(), p[1], packet); err != nil {
			t.Fatal(err)
		}
	}
	if w[0].Len() != 0 || w[2].Len() != 0 || len(w[1].packets(t)) != 5 {
		t.Fatal("mute leaked messages")
	}
	p[1].character.MutedUntil = time.Now().Add(-time.Second)
	say(t, s, p[1], "hello")
	if w[0].Len() == 0 {
		t.Fatal("expired mute still blocked")
	}
	say(t, s, p[0], "/unmute Bobby")
	if !p[1].character.MutedUntil.IsZero() {
		t.Fatal("unmute not saved")
	}
	assertProgressSaved(t, s, p[1])
}

func TestGMJailAtomicDestinationAndRelease(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	for _, id := range []uint16{10000, 10001} {
		s.Assets.Maps[id] = assets.Map{ID: id}
	}
	s.World = world.New(s.Assets)
	say(t, s, p[0], "/jail Bobby 2")
	target := p[1]
	if target.character.Map != 10000 || target.character.X != 600 || target.character.Y != 600 || target.ready || !time.Now().Before(target.character.MutedUntil) {
		t.Fatal("jail state")
	}
	assertProgressSaved(t, s, target)
	if !contains(w[0].packets(t), protocol.Builder{12}.U32(target.character.ID).U16(10000).U16(600).U16(600).U16(0).U8(0)) {
		t.Fatal("old map missed jail departure")
	}
	if err := s.worldCommand(context.Background(), target, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	say(t, s, p[0], "/unjail Bobby")
	if target.character.Map != 10001 || target.character.X != 800 || target.character.Y != 750 || !target.character.MutedUntil.IsZero() {
		t.Fatal("release state")
	}
	assertProgressSaved(t, s, target)
}

func TestGMHideSuppressesMovementAndLateArrival(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	say(t, s, p[0], "/hide")
	if !p[0].invisible || !contains(w[1].packets(t), protocol.Builder{12}.U32(p[0].character.ID).U16(0).U16(0).U16(0).U16(0).U8(0)) {
		t.Fatal("missing despawn")
	}
	s.broadcastWorld(p[0], []byte{16, 1})
	if w[1].Len() != 0 {
		t.Fatal("hidden movement broadcast")
	}
	p[1].ready = false
	delete(s.world, p[1].info.ID)
	if err := s.acknowledgeWorld(p[1]); err != nil {
		t.Fatal(err)
	}
	for _, packet := range w[1].packets(t) {
		if packet[0] == 4 {
			t.Fatal("hidden player spawned for entrant")
		}
	}
	say(t, s, p[0], "/unhide")
	packets := w[1].packets(t)
	if p[0].invisible || len(packets) < 3 || packets[0][0] != 4 {
		t.Fatal("unhide appearance", packets)
	}
	say(t, s, p[0], "/unhide")
	if p[0].invisible {
		t.Fatal("unhide toggled")
	}
}

func TestGMModerationAndPlayerEditSaveFailure(t *testing.T) {
	for _, command := range []string{"/god", "/clearinv", "/mute Bobby"} {
		t.Run(command, func(t *testing.T) {
			s, p, w := chatFixture(t)
			s.SetGMLevel(p[0].account.ID, 1)
			before := p[0].character.Clone()
			targetBefore := p[1].character.Clone()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := s.worldCommand(ctx, p[0], append([]byte{2, 2}, command...))
			if err == nil {
				t.Fatal("save succeeded with canceled context")
			}
			if !reflect.DeepEqual(*p[0].character, before) || !reflect.DeepEqual(*p[1].character, targetBefore) || w[0].Len() != 0 || w[1].Len() != 0 {
				t.Fatal("save failure published changes")
			}
		})
	}
}

func TestGMKickAllPreservesGMsAndReloadsPrivileges(t *testing.T) {
	s, p, _ := chatFixture(t)
	for _, player := range p {
		player.conn = &closeConn{}
	}
	if err := s.ChangeGMLevel(context.Background(), p[0].account.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeGMLevel(context.Background(), p[2].account.ID, 2); err != nil {
		t.Fatal(err)
	}
	// A character loading a warp is still a kickall recipient.
	delete(s.world, p[1].info.ID)
	p[1].ready = false
	say(t, s, p[0], "/kickall Maintenance")
	for i, player := range p {
		conn := player.conn.(*closeConn)
		if conn.closed != (i == 1) {
			t.Fatal("wrong kick recipient", i)
		}
	}
	if err := s.Store.SetGMLevel(context.Background(), p[2].account.ID, 0); err != nil {
		t.Fatal(err)
	}
	say(t, s, p[0], "/reload gm")
	if p[2].gmLevel.Load() != 0 || p[0].gmLevel.Load() != 1 {
		t.Fatal("GM reload did not revoke")
	}
}

func TestGMShutdownCountdownUsesGracefulRunCleanup(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	say(t, s, p[0], "/shutdown 10")
	deadline := s.shutdownAt
	say(t, s, p[0], "/shutdown 1")
	if !s.shutdownAt.Equal(deadline) {
		t.Fatal("duplicate rescheduled shutdown")
	}
	s.shutdownTick(deadline.Add(-3 * time.Second))
	if !bytes.Contains(bytes.Join(w[1].packets(t), nil), []byte("shutdown")) {
		t.Fatal("missing notice")
	}
	s.shutdownTick(deadline)
	select {
	case <-s.shutdown:
	default:
		t.Fatal("shutdown did not request stop")
	}
	// Run must bind and then exit normally even when shutdown was requested
	// immediately before it started; all background loops must be joined.
	cfg := config.Default()
	cfg.Login, cfg.World, cfg.Status = "127.0.0.1:0", "127.0.0.1:0", "127.0.0.1:0"
	runner := New(cfg, s.Store, s.Assets, s.Log)
	runner.Shutdown()
	done := make(chan error, 1)
	go func() { done <- runner.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run failed to exit gracefully")
	}
	for _, listener := range runner.listeners {
		if conn, err := listener.Accept(); err == nil {
			conn.Close()
			t.Fatal("listener survived shutdown")
		}
	}
}

func TestGMCombatVictoryAwardsOnce(t *testing.T) {
	s, p, _ := chatFixture(t)
	c := p[0]
	c.gmLevel.Store(1)
	s.Assets.NPCs = map[uint16]assets.NPC{42: {ID: 42, Name: "GM monster", Level: 4, HP: 200, Element: 1}}
	say(t, s, c, "/fight 65535")
	if c.battle != nil {
		t.Fatal("unknown battle started")
	}
	say(t, s, c, "/battle 42")
	run := c.battle
	if run == nil || run.b.Defenders[0].MaxHP != 200 {
		t.Fatal("battle did not use template")
	}
	say(t, s, c, "/killmonsters")
	s.worldMu.Lock()
	before := c.character.Clone()
	finished := run.b.Finished
	s.worldMu.Unlock()
	if !finished || before.Gold != 32 {
		t.Fatal("victory results", before.Gold)
	}
	say(t, s, c, "/winbattle")
	s.worldMu.Lock()
	if !reflect.DeepEqual(*c.character, before) {
		t.Fatal("double victory rewards")
	}
	s.worldMu.Unlock()
	deadline := time.Now().Add(4 * time.Second)
	for {
		s.worldMu.Lock()
		done := c.battle == nil
		s.worldMu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("battle exit did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	assertProgressSaved(t, s, c)
}

func TestGMInformationAndHelp(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	for _, cmd := range []string{"/who", "/whois Bobby", "/cmds"} {
		say(t, s, p[0], cmd)
		if w[0].Len() == 0 {
			t.Fatal("missing information", cmd)
		}
		w[0].Reset()
	}
	if w[1].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("information leaked")
	}
	say(t, s, p[0], fmt.Sprintf("/info %d", p[2].character.ID))
	if !bytes.Contains(bytes.Join(w[0].packets(t), nil), []byte("Carol")) {
		t.Fatal("ID lookup")
	}
}

func TestGMAllSkillsInstalledCatalog(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range []byte{1, 2, 3, 4} {
		t.Run(fmt.Sprint(element), func(t *testing.T) {
			s, p, _ := chatFixture(t)
			c := p[0]
			c.gmLevel.Store(1)
			c.character.Element = element
			s.Assets = catalog
			say(t, s, c, "/allskills 10")
			for _, id := range c.character.GMElementSkillIDs() {
				found := false
				for _, sk := range c.character.Skills {
					if sk.ID == id && sk.Grade == 10 {
						found = true
					}
				}
				if !found {
					t.Fatalf("SQL element %d missing GM skill %d", element, id)
				}
			}
			assertProgressSaved(t, s, c)
		})
	}
}

func TestGMFreshPetRecruitAndSaveFailure(t *testing.T) {
	s, p, w := petFixture(t)
	c := p[0]
	c.gmLevel.Store(1)
	before := c.character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.worldCommand(ctx, c, append([]byte{2, 2}, []byte("/pet 14156 Xiao Lan")...)); err == nil {
		t.Fatal("recruit succeeded with canceled save")
	}
	if !reflect.DeepEqual(*c.character, before) || len(c.pets.slots) != 0 || w[0].Len() != 0 {
		t.Fatal("failed recruit published state")
	}
	say(t, s, c, "/pet 14156 Xiao Lan")
	if len(c.character.Pets) != 1 || c.character.Pets[0].Name != "Xiao Lan" || c.character.ActivePet != 14156 || !c.character.Pets[0].Battle || c.pets.slot(14156) != 1 {
		t.Fatal("fresh recruit")
	}
	packets := w[0].packets(t)
	if len(packets) < 4 || !bytes.HasPrefix(packets[0], []byte{15, 1}) || !contains(packets, protocol.Builder{19, 1}.U32(14156)) {
		t.Fatal("native recruit/select", packets)
	}
	assertProgressSaved(t, s, c)
	before = c.character.Clone()
	c.character.HotelPets = append(c.character.HotelPets, c.character.Pets[0])
	blocked := c.character.Clone()
	say(t, s, c, "/pet 14156 Other")
	if !reflect.DeepEqual(*c.character, blocked) {
		t.Fatal("hotel companion duplicated")
	}
	*c.character = before
	s.Assets.NPCs[14161] = assets.NPC{ID: 14161, Name: "Roca", Type: 4}
	next := c.character.Clone()
	for _, id := range []uint32{100, 101, 102} {
		next.Pets = append(next.Pets, game.Pet{ID: id, Slot: byte(len(next.Pets) + 1), Level: 1})
	}
	*c.character = next
	say(t, s, c, "/pet 14161")
	if !reflect.DeepEqual(*c.character, next) {
		t.Fatal("full party changed")
	}
}

func TestGMJailSaveAndTargetDeliveryFailures(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	s.Assets.Maps[10000] = assets.Map{ID: 10000}
	s.World = world.New(s.Assets)
	before := p[1].character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.worldCommand(ctx, p[0], append([]byte{2, 2}, []byte("/jail Bobby")...)); err == nil {
		t.Fatal("jail ignored canceled save")
	}
	if !reflect.DeepEqual(*p[1].character, before) || w[1].Len() != 0 {
		t.Fatal("failed jail moved/muted target")
	}
	failed := &failedWorldConn{}
	p[1].conn = failed
	say(t, s, p[0], "/jail Bobby")
	if !failed.closed || p[1].character.Map != 10000 || !time.Now().Before(p[1].character.MutedUntil) {
		t.Fatal("failed delivery lost durable moderation")
	}
	if s.world[p[1].info.ID] != nil || !p[0].ready {
		t.Fatal("failed target remained published or disconnected GM")
	}
	assertProgressSaved(t, s, p[1])
}
