package game

import (
	"errors"
	"testing"
	"time"
)

func TestAcquisitionRequiresCompleteCapacity(t *testing.T) {
	for _, scenario := range []string{"full", "partial_stack", "partial_empty_slot", "locked_empty", "locked_stack", "different_metadata", "different_damage"} {
		t.Run(scenario, func(t *testing.T) {
			var bag Inventory
			for i := range bag {
				bag[i] = Item{ID: 2, Count: 1}
			}
			count, limit := 1, byte(MaxItemStack)
			switch scenario {
			case "partial_stack":
				bag[0] = Item{ID: 1, Count: MaxItemStack - 1}
				count = 2
			case "partial_empty_slot":
				bag[0] = Item{}
				count = MaxItemStack + 1
			case "locked_empty":
				bag[0] = Item{Locked: true}
			case "locked_stack":
				bag[0] = Item{ID: 1, Count: 1, Locked: true}
			case "different_metadata":
				bag[0] = Item{ID: 1, Count: 1, Metadata: [ItemMetadataBytes]byte{1}}
			case "different_damage":
				bag[0] = Item{ID: 1, Count: 1, Damage: 1}
			}
			before := bag
			adds, err := bag.Grant(Item{ID: 1}, count, limit)
			if !errors.Is(err, ErrInventoryFull) || len(adds) != 0 || bag != before {
				t.Fatalf("failed acquisition changed inventory: additions=%v error=%v", adds, err)
			}
		})
	}
	// A full bag can still accept the complete amount in a compatible stack.
	var bag Inventory
	for i := range bag {
		bag[i] = Item{ID: 2, Count: 1}
	}
	bag[0] = Item{ID: 1, Count: MaxItemStack - 2}
	adds, err := bag.Grant(Item{ID: 1}, 2, MaxItemStack)
	if err != nil || bag[0].Count != MaxItemStack || len(adds) != 1 || adds[0] != (Addition{Slot: 1, Count: 2}) {
		t.Fatalf("compatible stack capacity rejected: %v %v", adds, err)
	}
}

func TestQuestAcquisitionFullBagPreservesCompletion(t *testing.T) {
	c := Character{Quests: map[uint32]Quest{1: {ID: 1, State: InProgress, Step: 1}}}
	for i := range c.Bag {
		c.Bag[i] = Item{ID: 2, Count: 1}
	}
	c.Bag[0] = Item{}
	before := c.Clone()
	// One item fits, but the whole reward requires two slots.
	err := c.GrantQuestReward(1, []Item{{ID: 3, Count: 1}, {ID: 4, Count: 1}}, func(uint16) byte { return 1 }, time.Now())
	if !errors.Is(err, ErrInventoryFull) || c.Bag != before.Bag || c.Quests[1] != before.Quests[1] {
		t.Fatalf("partial quest acquisition or completion: %v", err)
	}
}
