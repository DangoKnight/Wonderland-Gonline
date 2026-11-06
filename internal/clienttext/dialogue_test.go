package clienttext

import "testing"

func TestNoaQuizAnswerDisplayCorrections(t *testing.T) {
	for _, tc := range []struct {
		id             uint32
		original, want string
	}{
		{57166, "Breillat*", "Breillat"},
		{57171, "Priest in church*", "Priest in church"},
		{57174, "Drawing*", "Drawing"},
		{57181, "Fire Element*", "Fire Element"},
		{57184, "Rey*", "Rey"},
	} {
		if got := Override(Talk, tc.id, Text, tc.original); got != tc.want {
			t.Fatal(tc.id, got)
		}
		for _, preserved := range []string{tc.want, "Custom answer*"} {
			if got := Override(Talk, tc.id, Text, preserved); got != preserved {
				t.Fatal("custom/idempotent text changed", tc.id, got)
			}
		}
		if got := Override(Talk, 1, Text, tc.original); got != tc.original {
			t.Fatal("unrelated dialogue changed", got)
		}
	}
}
