package battle

import "testing"

func TestNativeFormationQueuesRepeatedOccupants(t *testing.T) {
	// Black monster formation 10082:1: five records across three native slots.
	b := New(nil, []Enemy{{Template: 17921, FormationSlot: 2}, {Template: 17922, FormationSlot: 5}, {Template: 17923, FormationSlot: 7}, {Template: 17924, FormationSlot: 5}, {Template: 17925, FormationSlot: 7}})
	if len(b.Defenders) != 3 || b.Defenders[0].ID != 17921 || b.Defenders[1].ID != 17922 || b.Defenders[2].ID != 17923 || len(b.Reinforcements[0]) != 0 || len(b.Reinforcements[1]) != 1 || b.Reinforcements[1][0].Template != 17924 || len(b.Reinforcements[2]) != 1 || b.Reinforcements[2][0].Template != 17925 {
		t.Fatal("native slot associations lost", b.Defenders, b.Reinforcements)
	}
	oldX, oldY := b.Defenders[1].X, b.Defenders[1].Y
	b.Defenders[1].HP = 0
	packets := b.spawnReinforcements()
	if len(packets) == 0 || b.Defenders[1].ID != 17924 || b.Defenders[1].X != oldX || b.Defenders[1].Y != oldY || b.Defenders[2].ID != 17923 || len(b.Reinforcements[2]) != 1 {
		t.Fatal("wrong slot reinforced")
	}
	if history := b.DefenderHistory(); len(history) != 4 || history[0].ID != 17922 {
		t.Fatal("replaced occupant lost reward history")
	}
}

func TestNativeFormationConsecutiveDuplicatesAreNotInitialEnemies(t *testing.T) {
	b := New(nil, []Enemy{{Template: 17168, FormationSlot: 5}, {Template: 17168, FormationSlot: 5}, {Template: 17169, FormationSlot: 2}, {Template: 17169, FormationSlot: 2}, {Template: 17170, FormationSlot: 7}, {Template: 17170, FormationSlot: 7}, {Template: 17171, FormationSlot: 8}})
	if len(b.Defenders) != 4 || len(b.Reinforcements[0]) != 1 || len(b.Reinforcements[1]) != 1 || len(b.Reinforcements[2]) != 1 || len(b.Reinforcements[3]) != 0 {
		t.Fatal("duplicate slot spawned simultaneously")
	}
}
