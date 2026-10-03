package game

import (
	"math"
	"testing"
)

func TestResetAttributesRefundAndBonusExclusion(t *testing.T) {
	c := Character{Body: 4, Head: 2, Base: Attributes{10, 20, 30, 40, 50}, StatPoints: 7}
	if refund := c.ResetAttributes(); refund != 100 || c.StatPoints != 107 {
		t.Fatal(refund, c.StatPoints)
	}
	if c.Base != (Attributes{10, 10, 10, 10, 10}) || c.Attributes().Intelligence != 13 {
		t.Fatal("reset lost avatar bonus", c.Base, c.Attributes())
	}
	if refund := c.ResetAttributes(); refund != 0 || c.StatPoints != 107 {
		t.Fatal("repeat created points", refund, c.StatPoints)
	}
}

func TestResetAttributesBudgetsAndSaturation(t *testing.T) {
	for _, test := range []struct {
		base         Attributes
		points, want uint16
	}{
		{Attributes{1, 1, 1, 1, 1}, 7, 7},
		{Attributes{30, 0, 0, 0, 0}, 7, 7},
		{Attributes{60, 0, 0, 0, 0}, 7, 17},
		{Attributes{65535, 65535, 65535, 65535, 65535}, 0, math.MaxUint16},
		{Attributes{11, 10, 10, 10, 10}, math.MaxUint16, math.MaxUint16},
	} {
		c := Character{Base: test.base, StatPoints: test.points}
		got := c.ResetAttributes()
		if c.StatPoints != test.want || got != test.want-test.points || c.Base != (Attributes{10, 10, 10, 10, 10}) {
			t.Fatal(test, c.Base, c.StatPoints, got)
		}
	}
}
