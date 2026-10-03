package game

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
	"wonderland-go/internal/protocol"
)

func starterTestItems() map[uint16]ItemDefinition {
	return map[uint16]ItemDefinition{22005: {ID: 22005, EquipSlot: 1}, 21006: {ID: 21006, EquipSlot: 2}, 23001: {ID: 23001, EquipSlot: 4}, 24006: {ID: 24006, EquipSlot: 5}, 32176: {ID: 32176, Type: 23}}
}
func testCreatedCharacter(t *testing.T) Character {
	t.Helper()
	c, e := NewCharacter(10001, 1, "Player", Appearance{Body: 4, Head: 0, Element: 3, Hair: 0x1122, Skin: 0x3344, Clothing: 0x5566, Eyes: 0x7788, Base: Attributes{5, 5, 5, 5, 5}}, []StarterGrant{{32176, 50}}, starterTestItems(), time.Unix(0, 0))
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestStarterCharacter(t *testing.T) {
	c := testCreatedCharacter(t)
	if c.HP != 209 || c.MaxHP != 209 || c.SP != 121 || c.EXP != 6 || c.Map != 10017 || c.X != 1042 || c.Y != 1075 {
		t.Fatalf("wrong starter stats: %+v", c)
	}
	if c.Color1 != 0x33441122 || c.Color2 != 0x77885566 || c.Bag[0].Count != 50 || len(c.Skills) != 4 || c.Skills[0].ID != 11078 {
		t.Fatalf("wrong initial state: %+v", c)
	}
	if c.Quests[1].State != InProgress {
		t.Fatal("missing starter quest")
	}
}
func TestCreationFailsWithoutAssetsOrCapacity(t *testing.T) {
	a := Appearance{Body: 4, Element: 3}
	if _, e := NewCharacter(10001, 1, "Player", a, nil, nil, time.Now()); e == nil {
		t.Fatal("accepted missing outfit")
	}
	if _, e := NewCharacter(10001, 1, "Player", a, []StarterGrant{{32176, 2501}}, starterTestItems(), time.Now()); e == nil {
		t.Fatal("accepted oversized starter pack")
	}
}
func TestAppearanceDecoderAtomic(t *testing.T) {
	valid := protocol.Builder{}.U16(4).U16(0).U16(1).U16(2).U16(3).U16(4).U8(3).U8(5).U8(6).U8(7).U8(8).U8(9)
	for i := 0; i < len(valid); i++ {
		if _, _, e := DecodeAppearance(valid[:i]); e == nil {
			t.Fatalf("accepted %d byte prefix", i)
		}
	}
	a, _, e := DecodeAppearance(valid)
	if e != nil || a.Base.Strength != 5 || a.Base.Agility != 6 || a.Base.Wisdom != 7 || a.Base.Intelligence != 8 || a.Base.Constitution != 9 {
		t.Fatal(a, e)
	}
	withCode, _ := valid.String("secret1")
	_, code, e := DecodeAppearance(withCode)
	if e != nil || code != "secret1" {
		t.Fatal(code, e)
	}
	if _, _, e = DecodeAppearance(append(withCode, 99)); e == nil {
		t.Fatal("accepted trailing bytes")
	}
}
func TestCreationPacketLayouts(t *testing.T) {
	c := testCreatedCharacter(t)
	base, e := c.BaseStatsPacket(func(id uint16) (uint16, bool) {
		return map[uint16]uint16{15003: 188, 11016: 42, 11166: 43, 11056: 44}[id], true
	})
	if e != nil {
		t.Fatal(e)
	}
	if len(base) != 100 || binary.LittleEndian.Uint16(base[62:64]) != 4 {
		t.Fatalf("base layout: %x", base)
	}
	if !bytes.Equal(base[64:71], []byte{188, 0, 1, 0, 0, 0, 0}) {
		t.Fatalf("stunt wire alias: %x", base[64:71])
	}
	if binary.LittleEndian.Uint32(base[20:24]) != 6 || binary.LittleEndian.Uint16(base[24:26]) != 209 {
		t.Fatal("base field offsets")
	}
	equipment := c.EquipmentPacket()
	if len(equipment) != 2+4*21 || binary.LittleEndian.Uint16(equipment[2:4]) != 22005 {
		t.Fatal("equipment record size")
	}
	wantWarp := []byte{12, 0x11, 0x27, 0, 0, 0x21, 0x27, 0x12, 4, 0x33, 4, 0, 0, 0}
	if !bytes.Equal(c.WarpPacket(0), wantWarp) {
		t.Fatalf("warp: %x", c.WarpPacket(0))
	}
	self, e := c.AppearancePacket(false)
	if e != nil {
		t.Fatal(e)
	}
	if self[0] != 3 || self[5] != 4 || binary.LittleEndian.Uint32(self[15:19]) != c.Color1 {
		t.Fatalf("appearance: %x", self)
	}
	stats := c.StatPackets(nil)
	if stats[len(stats)-2][2] != 0x19 || stats[len(stats)-1][2] != 0x1a {
		t.Fatal("vitals must follow maxima")
	}
}

func TestNativeCreationTwoPasswordsGolden(t *testing.T) {
	// Reconstructed from native AC9:1's serializer (0x2c396c..0x2c3c66).
	// Both packed colour words contain decimal 444444444. The account
	// confirmation password is followed by a distinct deletion password.
	payload := []byte{4, 0, 0, 0, 0x1c, 0xaf, 0x7d, 0x1a, 0x1c, 0xaf, 0x7d, 0x1a, 3, 5, 5, 5, 5, 5, 8, 'l', 'o', 'g', 'i', 'n', '1', '2', '3', 9, 'd', 'e', 'l', 'e', 't', 'e', '4', '5', '6'}
	request, err := DecodeCharacterCreation(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !request.HasConfirmationPassword || request.ConfirmationPassword != "login123" || request.DeletionCode != "delete456" || request.Appearance.Body != 4 || request.Appearance.Element != 3 {
		t.Fatalf("native creation fields decoded incorrectly")
	}
	_, code, err := DecodeAppearance(payload)
	if err != nil || code != "delete456" {
		t.Fatal("legacy decoder must return the deletion password")
	}
	for _, bad := range [][]byte{payload[:len(payload)-1], append(append([]byte(nil), payload...), 0)} {
		if _, err := DecodeCharacterCreation(bad); err == nil {
			t.Fatal("malformed native credentials accepted")
		}
	}
}
