package server

import (
	"context"
	"testing"
	"wonderland-go/internal/assets"
)

func TestEventSkillRewardPersistenceAndUnknownPreflight(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "learn", false: "unknown"}[known], func(t *testing.T) {
			s, c, wire := eventFixture(t)
			ctx := context.Background()
			if known {
				s.Assets.Skills[15101] = assets.Skill{ID: 15101}
			}
			ev := assets.Event{ClickID: 77, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{
				evOp(1, 1, 1, 2, 0, 10), // No gold is delivered if the later skill is unknown.
				evOp(2, 11, 15101, 1, 0, 1),
			}}}}
			before := c.character.Gold
			if err := s.startEvent(ctx, c, 5, &ev, 0, false); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, skill := range chars[0].Skills {
				found = found || skill.ID == 15101
			}
			if found != known {
				t.Fatal("skill persistence", chars[0].Skills)
			}
			if known {
				if chars[0].Gold != before+10 || !contains(got, []byte{5, 16, 0, 0xfd, 0x3a, 1}) {
					t.Fatal("skill reward", got)
				}
			} else if chars[0].Gold != before {
				t.Fatal("unknown skill allowed earlier reward")
			}
		})
	}
}
