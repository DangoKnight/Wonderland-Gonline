package server

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func admissionFixture(t *testing.T) (*Server, []*Session, []*captureConn, *battleRun) {
	t.Helper()
	s, players, wires := worldFixture(t)
	for _, c := range players {
		c.character.Map = players[0].character.Map
		c.character.X, c.character.Y = players[0].character.X, players[0].character.Y
		if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	run := &battleRun{}
	if err := s.startBattle(players[0], run, []battle.Enemy{{Template: 17045, Level: 5, HP: 500}}); err != nil {
		t.Fatal(err)
	}
	run.timer.Stop()
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires, run
}

func entryRequest(c *Session, watch bool) []byte {
	mode := byte(5)
	if watch {
		mode = 4
	}
	id := c.character.ID
	return []byte{11, 2, mode, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24), 0, 0}
}

func TestBattleAdmissionJoinAndPet(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	pet := game.NewPet(14000, "Pet", 1, s.petTemplate(14000), s.Assets.Items)
	pet.Battle = true
	p[1].character.Pets = []game.Pet{pet}
	p[1].character.ActivePet = pet.ID
	run.b.Idle(run.members[0].self)
	pending := len(run.b.Pending)
	if err := s.battleStateCommand(p[1], entryRequest(p[0], false)); err != nil {
		t.Fatal(err)
	}
	m := run.member(p[1])
	if m == nil || m.self.X != 4 || m.self.Y != 3 || m.pet == nil || m.pet.X != 3 || m.pet.Y != 3 || len(run.members) != 2 || len(run.b.Pending) != pending+2 || run.b.Expected() != 3 || !m.self.Joining || !m.pet.Joining {
		t.Fatal("join changed formation or existing commands", m)
	}
	if p[1].battle != run || len(run.spectators) != 0 {
		t.Fatal("wrong membership")
	}
	if run.b.EscapeLeaders[m.self.ID] || !run.b.EscapeLeaders[run.members[0].self.ID] {
		t.Fatal("helper replaced original escape leadership")
	}
	packets := w[1].packets(t)
	if len(packets) == 0 || !bytes.Equal(packets[0][:2], []byte{11, 250}) || packets[0][4] != 5 || !contains(packets, []byte{11, 10, 1}) || contains(packets, []byte{52, 1}) {
		t.Fatal("missing joining snapshot", packets)
	}
	if len(w[0].packets(t)) == 0 {
		t.Fatal("existing combatant not notified")
	}
	if err := s.battleStateCommand(p[1], entryRequest(p[0], false)); err != nil || len(run.members) != 2 {
		t.Fatal("duplicate join", err)
	}
}

func TestBattleAdmissionSpectatorPlaybackAndExit(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	before := p[1].character.Clone()
	if err := s.battleStateCommand(p[1], entryRequest(p[0], true)); err != nil {
		t.Fatal(err)
	}
	if !run.spectators[p[1]] || run.member(p[1]) != nil || len(run.b.Attackers) != 1 || run.b.Expected() != 1 {
		t.Fatal("spectator became a fighter")
	}
	packets := w[1].packets(t)
	if packets[0][4] != 4 || packets[0][16] != 255 || packets[0][17] != 255 {
		t.Fatal("spectator must be off-grid", packets[0])
	}
	if err := s.battleCommand(p[1], []byte{50, 1, 4, 2, 2, 2, 17, 39}); err != nil || len(run.b.Pending) != 0 {
		t.Fatal("spectator submitted an action", err)
	}
	step := []byte{50, 6, 4, 2, 0}
	if !s.play(run, []battle.Step{{Packets: [][]byte{step}}}) || !contains(w[1].packets(t), step) {
		t.Fatal("spectator missed playback")
	}
	if err := s.battleCommand(p[1], []byte{11, 1, 3}); err != nil {
		t.Fatal(err)
	}
	if p[1].battle != nil || len(run.spectators) != 0 || run.b.Finished || p[0].battle != run || !reflect.DeepEqual(before, *p[1].character) {
		t.Fatal("spectator exit changed gameplay")
	}
}

func TestBattleAdmissionQueuesDuringPlaybackAndCancels(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	run.b.Processing = true
	if err := s.battleStateCommand(p[1], entryRequest(p[0], false)); err != nil {
		t.Fatal(err)
	}
	if len(run.admissions) != 1 || len(run.members) != 1 || len(run.b.Attackers) != 1 || contains(w[1].packets(t), []byte{11, 10, 1}) {
		t.Fatal("joined immutable playback")
	}
	s.abandonBattle(p[1])
	if len(run.admissions) != 0 || p[1].battle != nil || run.b.Finished {
		t.Fatal("queued disconnect damaged battle")
	}
	if err := s.battleStateCommand(p[1], entryRequest(p[0], true)); err != nil {
		t.Fatal(err)
	}
	run.b.Processing = false
	s.admitWaitingBattleClients(run)
	if !run.spectators[p[1]] || len(run.admissions) != 0 {
		t.Fatal("queued spectator was not admitted")
	}
	s.abandonBattle(p[0])
	if p[1].battle != nil || !run.b.Finished || len(run.spectators) != 0 {
		t.Fatal("last combatant left spectator locked")
	}
}

func TestBattleAdmissionRejectsUnavailableTargets(t *testing.T) {
	for _, mode := range []string{"distance", "map", "disabled", "full", "finished"} {
		t.Run(mode, func(t *testing.T) {
			s, p, _, run := admissionFixture(t)
			switch mode {
			case "distance":
				p[1].character.X += 171
			case "map":
				p[1].character.Map++
			case "disabled":
				prefs := p[0].character.Preferences()
				prefs.JoinAllowed = false
				p[0].character.Settings = &prefs
			case "full":
				for i := 1; i < 4; i++ {
					f := battle.PlayerFighter(p[0].character, s.Assets.Items, i)
					f.ID += uint32(i)
					run.b.Admit(f, nil)
				}
			case "finished":
				run.b.Finished = true
			}
			if err := s.battleStateCommand(p[1], entryRequest(p[0], false)); err != nil || p[1].battle != nil || len(run.members) != 1 {
				t.Fatal("invalid admission", err)
			}
		})
	}
}

func TestBattleAdmissionSpectatorFinishNoRewards(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	if err := s.battleStateCommand(p[1], entryRequest(p[0], true)); err != nil {
		t.Fatal(err)
	}
	before := p[1].character.Clone()
	w[1].Reset()
	s.closeBattleObservers(run, []byte{11, 12, 1})
	packets := w[1].packets(t)
	if p[1].battle != nil || !reflect.DeepEqual(before, *p[1].character) || !contains(packets, []byte{11, 12, 1}) || !contains(packets, []byte{20, 8}) {
		t.Fatal("spectator cleanup", packets)
	}
	if got := battlePresenceState(p[0]); got[0] != protocol.CommandBattleState {
		t.Fatal(got)
	}
}

func TestBattleAdmissionSpectatorRoundChainIsPublicOnly(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	p[1].combatCapabilities = protocol.CombatCapabilityRoundChain
	if err := s.battleStateCommand(p[1], entryRequest(p[0], true)); err != nil {
		t.Fatal(err)
	}
	w[1].Reset()
	public := []byte{50, 6, 4, 2, 0}
	private := []byte{23, 9, 1, 1}
	if !s.play(run, []battle.Step{{Packets: [][]byte{public}, OwnerPackets: map[uint32][][]byte{p[0].character.ID: {private}}}}) {
		t.Fatal("playback stopped")
	}
	packets := w[1].packets(t)
	steps := decodeRoundPackets(t, packets)
	if len(steps) != 1 || len(steps[0].Packets) != 1 || !bytes.Equal(steps[0].Packets[0], public) || contains(packets, public) || contains(packets, private) {
		t.Fatal("spectator chain leaked or duplicated packets", packets)
	}
}

func TestBattleAdmissionJoinsDefendingPvPSide(t *testing.T) {
	s, p, _, run := admissionFixture(t)
	run.b.Finished = true
	p[0].battle = nil
	if err := s.startPvP(p[0], p[1]); err != nil {
		t.Fatal(err)
	}
	run = p[0].battle
	run.timer.Stop()
	if err := s.battleStateCommand(p[2], entryRequest(p[1], false)); err != nil {
		t.Fatal(err)
	}
	m := run.member(p[2])
	if m == nil || m.self.Side != battle.Defender || m.self.X != 1 || m.self.Y != 3 || len(run.b.Defenders) != 2 {
		t.Fatal("joined wrong PvP side", m)
	}
}

func TestBattleAdmissionWaitsUntilNextCommandPhase(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	if err := s.battleStateCommand(p[1], entryRequest(p[0], false)); err != nil {
		t.Fatal(err)
	}
	m := run.member(p[1])
	if !m.self.Joining || len(run.b.Pending) != 1 {
		t.Fatal("join must reserve an idle action")
	}
	if ack := run.b.Submit(s.combatRules(run), p[1].character.ID, 1, []byte{m.self.X, m.self.Y, 2, 2, 17, 39}); ack != nil {
		t.Fatal("joining player acted before next phase")
	}
	run.b.Timeout()
	_, outcome := run.b.Round(s.combatRules(run))
	if outcome != battle.Continue || m.self.Joining {
		t.Fatal("join did not become eligible next round")
	}
	w[1].Reset()
	prompt, err := s.combatPrompt(m, run.b.NextRound(m.self.ID))
	if err != nil || len(prompt) != 2 {
		t.Fatal("missing next-round prompt", err)
	}
}

func TestBattleAdmissionQueuedFighterReadyAtCommandPhase(t *testing.T) {
	s, p, w, run := admissionFixture(t)
	run.b.Processing = true
	if err := s.battleStateCommand(p[1], entryRequest(p[0], false)); err != nil {
		t.Fatal(err)
	}
	run.b.Processing = false
	s.admitWaitingBattleClients(run)
	m := run.member(p[1])
	if m == nil || m.self.Joining || len(run.b.Pending) != 0 {
		t.Fatal("queued entry delayed an extra round")
	}
	if contains(w[1].packets(t), []byte{52, 1}) {
		t.Fatal("entry sent a duplicate command-phase prompt")
	}
}
