package game

import (
	"math"
	"testing"
)

func TestCriticalNativeChanceAndOverflow(t *testing.T) {
	c, e := ParseCriticalHits([]byte(`{"damage_multiplier":1.5,"items":[{"item_id":1,"chance_percent":170}]}`))
	if e != nil {
		t.Fatal(e)
	}
	eq := [6]uint16{1}
	if n, critical := c.Damage(3, eq, "attack", 99); n != 4 || !critical {
		t.Fatalf("%d %t", n, critical)
	}
	if n, critical := c.Damage(math.MaxInt32, eq, "attack", 0); n != math.MaxInt32 || !critical {
		t.Fatal(n)
	}
	if _, critical := c.Damage(10, eq, "skill", 0); critical {
		t.Fatal("skill incorrectly critical")
	}
	if _, critical := c.Damage(10, [6]uint16{}, "attack", 0); critical {
		t.Fatal("pet inherited player equipment")
	}
}
