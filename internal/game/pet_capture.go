package game

// NewCapturedPet applies the wiki's initial-attribute rule to wild monsters.
// Story recruitment continues to use NewPet and its authored attributes.
func NewCapturedPet(id uint32, name string, slot byte, t PetTemplate, items map[uint16]ItemDefinition, roll func(int) int, options ...PetGrowthOptions) Pet {
	p := NewPet(id, name, slot, t, items)
	values := []*uint16{&p.Base.Strength, &p.Base.Constitution, &p.Base.Intelligence, &p.Base.Wisdom, &p.Base.Agility}
	odd := 0
	for _, value := range values {
		odd += int(*value % 2)
		*value /= 2
	}
	for bonus := 0; bonus < odd/2; bonus++ {
		p.grow(t, true, roll, options...)
	}
	p.Normalize(items, true, t)
	return p
}

const (
	MonsterNetItemID              uint16 = 25218
	PetNetItemID                  uint16 = 25225
	CaptureNetEquipmentSlot              = 6
	CaptureNetLevelAllowance             = 10
	OrdinaryCaptureLevelAllowance        = 5
	CaptureNetDamagePerUse               = 10
	CaptureNetExhaustedDamage            = 100
)

func (c Character) CaptureNet() (Item, bool) {
	net := c.Equipment[CaptureNetEquipmentSlot-1]
	return net, !net.Empty() && !net.Locked && net.Damage < CaptureNetExhaustedDamage && (net.ID == MonsterNetItemID || net.ID == PetNetItemID)
}
