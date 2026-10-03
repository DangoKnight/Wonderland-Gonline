package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func TestSettingsNativePacketsAndPersistence(t *testing.T) {
	s, players, wires := partyFixture(t)
	c := players[0]
	tradeDo(t, s, c, []byte{33, 2})
	if p := wires[0].packets(t); len(p) != 1 || !bytes.Equal(p[0], []byte{33, 2, 1, 1, 1, 1, 31, 0}) {
		t.Fatal("native defaults", p)
	}
	cases := []struct{ in, out []byte }{
		{[]byte{16, 1, 0}, []byte{16, 1, 0}},
		{[]byte{33, 1, 1}, []byte{33, 1, 1, 1}},
		{[]byte{16, 2}, []byte{16, 2, 1}},
		{[]byte{33, 1, 4}, []byte{33, 1, 4, 1}},
		{[]byte{16, 3, 1}, []byte{16, 3, 1}},
		{[]byte{33, 1, 2}, []byte{33, 1, 2, 1}},
		{[]byte{16, 4, 2}, []byte{16, 4, 2}},
		{[]byte{33, 5}, []byte{33, 5, 1}},
		{[]byte{33, 5, 0}, []byte{33, 5, 0}},
		{[]byte{33, 1, 3, 18}, nil},
		{[]byte{33, 2}, []byte{33, 2, 1, 1, 1, 1, 18, 0}},
	}
	for _, tt := range cases {
		wires[0].Reset()
		tradeDo(t, s, c, tt.in)
		p := wires[0].packets(t)
		if tt.out == nil {
			if len(p) != 0 {
				t.Fatal("channel update sent an invented ACK", p)
			}
			continue
		}
		if len(p) != 1 || !bytes.Equal(p[0], tt.out) {
			t.Fatal("settings wire", tt.in, p)
		}
	}
	if c.walkMode != 2 {
		t.Fatal("walk setting not adopted")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(chars) != 1 {
		t.Fatal(err)
	}
	if chars[0].Preferences() != c.character.Preferences() {
		t.Fatal("settings not saved")
	}
	packets, err := s.worldEntryPackets(chars[0], c.view, newPetRoster())
	if err != nil || !contains(packets, []byte{33, 2, 1, 1, 1, 1, 18, 0}) {
		t.Fatal("login used fixed preferences", err)
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("settings leaked to peers")
	}
}

func TestSettingsMalformedAndSaveFailure(t *testing.T) {
	s, players, wires := partyFixture(t)
	c := players[0]
	before := c.character.Preferences()
	for _, p := range [][]byte{{16}, {16, 1, 1, 1}, {33}, {33, 1}, {33, 1, 3}, {33, 1, 1, 1}, {33, 2, 1}, {33, 5, 1, 1}} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatal("malformed preferences accepted", p)
		}
	}
	for _, p := range [][]byte{{16, 99}, {33, 99}, {33, 1, 99}} {
		if err := s.dispatch(context.Background(), c, p); err != ErrUnsupported {
			t.Fatal("unknown preferences accepted", p, err)
		}
	}
	if c.character.Preferences() != before || wires[0].Len() != 0 {
		t.Fatal("invalid packet mutated preferences")
	}
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{33, 1, 4}); err == nil {
		t.Fatal("failed save ignored")
	}
	if c.character.Preferences() != before || c.character.Settings != nil || wires[0].Len() != 0 {
		t.Fatal("failed save adopted or acknowledged")
	}
}

func TestSettingsTeamRequestsAndPendingAccept(t *testing.T) {
	for _, toggle := range [][]byte{{16, 3, 1}, {33, 1, 2}} {
		s, players, wires := partyFixture(t)
		target, requester := players[0], players[1]
		tradeDo(t, s, requester, protocol.Builder{13, 1}.U32(target.character.ID))
		if len(target.partyRequests) != 1 {
			t.Fatal("pending request missing")
		}
		tradeDo(t, s, target, toggle)
		if len(target.partyRequests) != 0 {
			t.Fatal("auto reject retained pending invitation")
		}
		wires[0].Reset()
		tradeDo(t, s, requester, protocol.Builder{13, 1}.U32(target.character.ID))
		tradeDo(t, s, target, protocol.Builder{13, 3, 1}.U32(requester.character.ID))
		if target.party != nil || requester.party != nil || wires[0].Len() != 0 {
			t.Fatal("reject-team preference bypassed")
		}
		tradeDo(t, s, target, []byte{16, 3, 0})
		tradeDo(t, s, target, protocol.Builder{13, 3, 1}.U32(requester.character.ID))
		if target.party != nil {
			t.Fatal("stale invitation accepted after enabling")
		}
		tradeDo(t, s, requester, protocol.Builder{13, 1}.U32(target.character.ID))
		tradeDo(t, s, target, protocol.Builder{13, 3, 1}.U32(requester.character.ID))
		if target.party == nil || target.party != requester.party {
			t.Fatal("reenabling requests did not permit fresh invitation")
		}
		tradeDo(t, s, target, []byte{16, 3, 1})
		if target.party == nil {
			t.Fatal("request preference dissolved existing team")
		}
	}
}

func TestSettingsTradeLockCancelsAndBlocksRequests(t *testing.T) {
	for _, active := range []bool{false, true} {
		s, players, wires := tradeFixture(t)
		a, b := players[0], players[1]
		if active {
			openTrade(t, s, players, wires)
		} else {
			tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
		}
		tradeDo(t, s, b, []byte{16, 2, 1})
		if a.trade != nil || b.trade != nil || b.tradeRequest != nil || !contains(wires[0].packets(t), []byte{25, 2, 2}) {
			t.Fatal("trade lock retained active/pending trade")
		}
		tradeDo(t, s, b, []byte{25, 2, 1})
		tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
		if b.tradeRequest != nil {
			t.Fatal("locked recipient received trade request")
		}
		tradeDo(t, s, b, protocol.Builder{25, 1}.U32(a.character.ID))
		if a.tradeRequest != nil {
			t.Fatal("locked requester initiated trade")
		}
		tradeDo(t, s, b, []byte{33, 1, 4})
		openTrade(t, s, players, wires)
		tradeDo(t, s, b, []byte{33, 1, 4})
		if a.trade != nil || b.trade != nil {
			t.Fatal("AC33 trade toggle failed to cancel")
		}
		saved := tradeStored(t, s, players)
		if saved[1].Preferences().TradeAllowed || saved[0].Gold != 100 || saved[1].Gold != 200 {
			t.Fatal("lock state or unchanged balances not saved")
		}
	}
}

func TestSettingsUpdateDuringBattleAndBeforeMapAck(t *testing.T) {
	s, players, wires := partyFixture(t)
	c := players[0]
	c.ready = false
	tradeDo(t, s, c, []byte{16, 1, 0})
	if c.character.Preferences().PKAllowed || !contains(wires[0].packets(t), []byte{16, 1, 0}) {
		t.Fatal("settings before map ACK refused")
	}
	c.ready = true
	c.battle = &battleRun{}
	tradeDo(t, s, c, []byte{33, 1, 1})
	if !c.character.Preferences().PKAllowed {
		t.Fatal("battle prevented settings update")
	}
	c.battle = nil
}

func TestSettingsCloneCannotMutateOriginal(t *testing.T) {
	c := game.Character{Settings: &game.ClientSettings{PKAllowed: true, JoinAllowed: true, TradeAllowed: true, Channels: 31}}
	copy := c.Clone()
	copy.Settings.TradeAllowed = false
	if !c.Settings.TradeAllowed {
		t.Fatal("clone shares settings")
	}
}
