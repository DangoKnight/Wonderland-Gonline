package inventory

import (
	"bytes"
	"strings"
	"testing"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func useFixture(id uint16) (*Form, *[][]byte) {
	f := &Form{State: &State{Items: map[uint16]assets.NativeItem{}}, Stats: &world.Stats{Potential: 5}}
	f.Visible = true
	f.State.Bag[0] = game.Item{ID: id, Count: 3}
	var sent [][]byte
	f.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	return f, &sent
}
func TestItemUseTargetedPacketsAndValidation(t *testing.T) {
	f, sent := useFixture(32011)
	item := f.State.Bag[0]
	f.State.Pets[2] = UsePet{ID: 12032, Name: []byte("Robinson")}
	if !f.UseSelected(1, 2, 3, item) || len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, 15, 1, 2, 3, 0}) {
		t.Fatalf("pet recovery request %x", *sent)
	}
	if f.State.Bag[0] != item {
		t.Fatal("request consumed locally")
	}
	for _, input := range [][3]byte{{0, 1, 0}, {1, 0, 0}, {1, 4, 0}, {1, 1, 5}, {1, 1, 1}} {
		if f.UseSelected(input[0], input[1], input[2], item) {
			t.Fatalf("invalid request accepted %v", input)
		}
	}
	f.State.Bag[0].Count--
	if f.UseSelected(1, 1, 0, item) {
		t.Fatal("stale stack accepted")
	}
	f.State.Bag[0] = item
	f.State.Bag[0].Locked = true
	if f.UseSelected(1, 1, 0, f.State.Bag[0]) {
		t.Fatal("locked item accepted")
	}
	f.State.Bag[0] = item
	f.CanAct = func() bool { return false }
	if f.UseSelected(1, 1, 0, item) {
		t.Fatal("blocked action accepted")
	}
	if len(*sent) != 1 {
		t.Fatal("invalid actions sent packets")
	}
}
func TestItemUsePotentialResults(t *testing.T) {
	f, sent := useFixture(game.ItemPotentialPill)
	var notices []string
	f.Notice = func(s string) { notices = append(notices, s) }
	item := f.State.Bag[0]
	if !f.UseSelected(1, 1, 0, item) || !bytes.Equal((*sent)[0], []byte{23, 126, 1, 0}) {
		t.Fatalf("pill request %x", *sent)
	}
	if f.UseSelected(1, 1, 0, item) || f.Stats.Potential != 5 || f.State.Bag[0] != item {
		t.Fatal("duplicate or optimistic pill")
	}
	if f.PotentialReply([]byte{23, 213, 1, 2, 4}) || f.PotentialReply([]byte{23, 213, 1, 0, 13}) {
		t.Fatal("mismatched/malformed reply accepted")
	}
	if !f.PotentialReply([]byte{23, 213, 1, 0, 4}) || f.Stats.Potential != 4 || !strings.Contains(notices[len(notices)-1], "failed") {
		t.Fatal("processed failure treated as success")
	}
	if !f.UseSelected(1, 1, 0, item) || !f.PotentialReply([]byte{23, 213, 0, 0, 0}) || f.Stats.Potential != 4 {
		t.Fatal("rejection changed potential")
	}
	f.State.Pets[3] = UsePet{ID: 12032, Potential: 10}
	f.State.Bag[0] = game.Item{ID: game.ItemGoldenPotentialPill, Count: 1}
	item = f.State.Bag[0]
	if f.UseSelected(1, 1, 0, item) || !f.UseSelected(1, 1, 4, item) {
		t.Fatal("golden pill minimum/target")
	}
	if !bytes.Equal((*sent)[len(*sent)-1], []byte{23, 126, 1, 4}) {
		t.Fatal("pet pill request")
	}
	if !f.PotentialReply([]byte{23, 213, 1, 4, 10}) || f.State.Pets[3].Potential != 10 || f.Stats.Potential != 4 {
		t.Fatal("pet failure changed player")
	}
	f.State.Pets[3].Potential = 12
	if f.UseSelected(1, 1, 4, item) {
		t.Fatal("maximum potential allowed")
	}
}
func TestItemUsePetListGoldenAndAtomicValidation(t *testing.T) {
	// Native record: slot 3, NPC 12032, name Roca, three skill records,
	// six equipment records and rebirth/potential tail.
	packet := []byte{15, 8, 3, 0, 47, 6, 0, 0, 0, 1, 181, 0, 0, 0, 100, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 0, 60, 1, 0, 0, 4, 'R', 'o', 'c', 'a'}
	packet = append(packet, make([]byte, 15+126)...)
	packet = append(packet, 0, 0, 1, 7, 0, 0, 0, 0, 0, 0, 0)
	// The first skill at byte 36 carries grade 2 and 50 EXP.
	packet[36], packet[37] = 2, 50
	// First worn record starts at byte 51; its forge byte is native +15.
	packet[51], packet[53], packet[66] = 100, 4, 7
	var s State
	if !s.ApplyPetList(packet) || s.Pets[2].ID != 12032 || string(s.Pets[2].Name) != "Roca" || s.Pets[2].Potential != 7 {
		t.Fatalf("native pet roster %#v", s.Pets)
	}
	if skill := s.Pets[2].Skills[0]; skill.Grade != 2 || skill.Exp != 50 {
		t.Fatalf("pet skill snapshot %#v", skill)
	}
	badGrade := append([]byte(nil), packet...)
	badGrade[36] = 11
	if s.ApplyPetList(badGrade) || s.Pets[2].Skills[0].Grade != 2 {
		t.Fatal("malformed skill grade replaced roster")
	}
	if equipment := s.Pets[2].Equipment[0]; equipment.ID != 100 || equipment.Damage != 4 || equipment.Forge() != 7 {
		t.Fatalf("pet equipment metadata %#v", equipment)
	}
	for _, bad := range [][]byte{packet[:len(packet)-1], append(append([]byte(nil), packet...), packet[2:]...)} {
		if s.ApplyPetList(bad) || s.Pets[2].Potential != 7 {
			t.Fatal("malformed list partially replaced roster")
		}
	}
	packet[32] = 'X'
	if string(s.Pets[2].Name) != "Roca" {
		t.Fatal("name aliases packet buffer")
	}
	s.Reset(nil)
	if s.Pets[2].ID != 0 {
		t.Fatal("character switch retained pets")
	}
}

func TestInventorySelectionDirectUseAndPetReceipts(t *testing.T) {
	f, sent := useFixture(32011)
	f.State.Items[32011] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 8, Status: [2]uint16{25}, Values: [2]int32{400}}}
	f.State.Pets[2] = UsePet{ID: 12032, Stats: world.Stats{HP: 50, MaxHP: 200}}
	f.SelectNext(1)
	if f.Selected != 3 {
		t.Fatal("selector skipped owned pet")
	}
	f.Use(1, false)
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, 15, 1, 1, 3, 0}) || f.UseDialog != nil {
		t.Fatalf("selected pet consume %x", *sent)
	}
	f.SelectNext(1)
	if f.Selected != 0 {
		t.Fatal("selector did not wrap to player")
	}
	f.Stats.HP, f.Stats.MaxHP = 100, 100
	f.Use(1, false)
	if len(*sent) != 1 {
		t.Fatal("full HP consumed HP-only item")
	}
	f.State.Items[100] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 1, Status: [2]uint16{207}, Values: [2]int32{150}}}
	f.State.Bag[1] = game.Item{ID: 100, Count: 1}
	f.Select(3)
	f.Use(2, false)
	if !bytes.Equal((*sent)[1], []byte{23, 17, 3, 2}) || !f.Equipment()[0].Empty() {
		t.Fatal("pet wear request or optimistic wear")
	}
	if handled, valid := f.State.Apply([]byte{23, 23, 3, 2}); !handled || !valid || f.Equipment()[0].ID != 100 || !f.State.Bag[1].Empty() {
		t.Fatal("pet equip receipt")
	}
	before := f.State.Pets[2].Equipment
	if _, valid := f.State.Apply([]byte{23, 22, 3, 1, 1}); valid || f.State.Pets[2].Equipment != before {
		t.Fatal("invalid pet unequip changed equipment")
	}
	if _, valid := f.State.Apply([]byte{23, 22, 3, 1, 2}); !valid || !f.Equipment()[0].Empty() || f.State.Bag[1].ID != 100 {
		t.Fatal("pet unequip receipt")
	}
	if !f.State.ApplyPetStat([]byte{8, 2, 4, 3, 0, 25, 1, 150, 0, 0, 0, 0, 0, 0, 0}) || f.DisplayStats().HP != 150 || f.Stats.HP != 100 {
		t.Fatal("pet stat receipt overwrote player")
	}
	if f.State.ApplyPetStat([]byte{8, 2, 4, 3, 0, 25, 3, 200, 0, 0, 0, 0, 0, 0, 0}) || f.DisplayStats().HP != 150 {
		t.Fatal("malformed pet stat receipt mutated state")
	}
}

func TestInventoryRightClickDoesNotUseItem(t *testing.T) {
	f, sent := useFixture(32011)
	f.State.Items[32011] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 8}}
	c := &slotControl{form: f, slot: 1}
	c.Env = &seui.Env{}
	c.Visible, c.Enabled = true, true
	c.RightUp(0, 0, 0)
	if len(*sent) != 0 || f.UseDialog != nil {
		t.Fatal("right-click consumed or opened a consumable confirmation")
	}
	c.DblClick()
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{23, 15, 1, 1, 0, 0}) {
		t.Fatalf("double-click no longer uses one item: %x", *sent)
	}
	f.State.Bag[0] = game.Item{ID: game.ItemPotentialPill, Count: 1}
	c.RightUp(0, 0, 0)
	if len(*sent) != 1 || f.UseDialog != nil {
		t.Fatal("right-click entered the Potential Pill use path")
	}
}
