package server

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestPetFoodCompoundingNativeGuaranteedAndPersistent(t *testing.T) {
	for _, command := range []byte{14, 87, 101} {
		t.Run("AC23:"+strconv.Itoa(int(command)), func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			// Deliberately no Junior/Superior skill; a zero roll would cause
			// ordinary alchemy to fail. Only a food-selection roll is allowed.
			s.Assets.PetFoods = map[uint16]assets.PetFood{100: {}, 200: {}, 300: {}}
			next := c.character.Clone()
			next.Skills = []game.LearnedSkill{{ID: game.AlchemyPrimarySkill, Grade: 1, EXP: 3}}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			beforeSkills := append([]game.LearnedSkill(nil), c.character.Skills...)
			calls := 0
			s.alchemyRandom = func(n int) (int, error) {
				calls++
				if n != 3 {
					t.Fatalf("non-food/failure roll: %d", n)
				}
				return 0, nil
			}
			wires[0].Reset()
			if err := s.dispatch(context.Background(), c, []byte{23, command, 2, 5, 9}); err != nil {
				t.Fatal(err)
			}
			want := [][]byte{{23, 9, 5, 1}, {23, 9, 9, 1}, append([]byte{23, 8, 5, 100, 0, 1}, make([]byte, 28)...), {23, 13, 100, 0, 1, 5}}
			got := wires[0].packets(t)
			if !reflect.DeepEqual(got, want) || calls != 1 || !reflect.DeepEqual(c.character.Skills, beforeSkills) {
				t.Fatal("wrong guaranteed recipe or alchemy EXP", got, calls, c.character.Skills)
			}
			chars, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || chars[0].Bag != c.character.Bag || c.character.Bag[4] != (game.Item{ID: 100, Count: 1}) || !c.character.Bag[8].Empty() {
				t.Fatal("not committed", err)
			}
		})
	}
}

func TestPetFoodCompoundingRejectsMixedAndRollsBack(t *testing.T) {
	for _, reason := range []string{"mixed", "three foods", "locked", "full bag", "random", "save"} {
		t.Run(reason, func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			s.Assets.PetFoods = map[uint16]assets.PetFood{100: {}, 200: {}, 300: {}}
			next := c.character.Clone()
			request := []byte{23, 14, 2, 5, 9}
			switch reason {
			case "mixed":
				delete(s.Assets.PetFoods, 200)
			case "three foods":
				next.Bag[0] = game.Item{ID: 300, Count: 1}
				request = []byte{23, 14, 3, 5, 9, 1}
			case "full bag":
				for i := range next.Bag {
					next.Bag[i] = game.Item{ID: 100, Count: 2}
				}
				next.Bag[8].ID = 200
			case "random":
				s.alchemyRandom = func(int) (int, error) { return 0, errors.New("entropy unavailable") }
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			savedBag := c.character.Bag
			if reason == "locked" {
				c.character.Bag[4].Locked = true
			}
			before := c.character.Bag
			wires[0].Reset()
			ctx := context.Background()
			if reason == "save" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := s.dispatch(ctx, c, request)
			if (reason == "save" || reason == "random") && err == nil {
				t.Fatal("missing infrastructure error")
			}
			chars, readErr := s.Store.Characters(context.Background(), c.account.ID)
			if readErr != nil || chars[0].Bag != savedBag || c.character.Bag != before {
				t.Fatal("rejected recipe spent food", err, readErr)
			}
			for _, p := range wires[0].packets(t) {
				if bytes.HasPrefix(p, []byte{23, 9}) || bytes.HasPrefix(p, []byte{23, 8}) || bytes.HasPrefix(p, []byte{23, 13}) {
					t.Fatal("published failed transaction", p)
				}
			}
		})
	}
}

func TestPetFoodCompoundingCompatibilityAndSameFood(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	s.Assets.PetFoods = map[uint16]assets.PetFood{100: {}, 200: {}, 300: {}}
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 100, Count: 2}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.dispatch(context.Background(), c, []byte{40, 7, 5, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[0].ID != 300 || c.character.Bag[8].Count != 1 || !c.character.Bag[4].Empty() {
		t.Fatal("same-food pair", c.character.Bag)
	}
	packets := wires[0].packets(t)
	if !bytes.Equal(packets[len(packets)-1], []byte{40, 7, 1, 44, 1}) {
		t.Fatal("compatibility receipt", packets)
	}
}
