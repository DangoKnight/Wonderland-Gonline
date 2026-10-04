package game

// ClearItemLocks removes process-owned reservations from canonical durable state.
func (c *Character) ClearItemLocks() {
	for i := range c.Bag {
		c.Bag[i].Locked = false
		c.Storage[i].Locked = false
	}
	for i := range c.Equipment {
		c.Equipment[i].Locked = false
	}
	for _, pets := range [][]Pet{c.Pets, c.ReservePets, c.HotelPets} {
		for i := range pets {
			for j := range pets[i].Equipment {
				pets[i].Equipment[j].Locked = false
			}
		}
	}
}
func PreserveItemLocks(before Character, next *Character) error {
	check := func(a Item, b *Item) error {
		locked := a.Locked
		a.Locked = false
		value := *b
		value.Locked = false
		if locked && a != value {
			return ErrItemLocked
		}
		b.Locked = locked
		return nil
	}
	for i := range before.Bag {
		if err := check(before.Bag[i], &next.Bag[i]); err != nil {
			return err
		}
		if err := check(before.Storage[i], &next.Storage[i]); err != nil {
			return err
		}
	}
	for i := range before.Equipment {
		if err := check(before.Equipment[i], &next.Equipment[i]); err != nil {
			return err
		}
	}
	for group, pets := range [][]Pet{before.Pets, before.ReservePets, before.HotelPets} {
		targets := [][]Pet{next.Pets, next.ReservePets, next.HotelPets}[group]
		for i, pet := range pets {
			for j, item := range pet.Equipment {
				if !item.Locked {
					continue
				}
				if i >= len(targets) || pet.ID != targets[i].ID || pet.Slot != targets[i].Slot {
					return ErrItemLocked
				}
				if err := check(item, &targets[i].Equipment[j]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
