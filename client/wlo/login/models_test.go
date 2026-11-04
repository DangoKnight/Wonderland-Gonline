package login

import "testing"

func TestNativeModelAppearances(t *testing.T) {
	// Independent native FUN_00484fcc outputs, including Breillat's model 13.
	want := [][2]byte{{2, 0}, {2, 1}, {1, 0}, {4, 2}, {4, 1}, {3, 0}, {3, 1}, {4, 4}, {4, 5}, {4, 0}, {4, 6}, {3, 2}, {4, 3}}
	for model := 0; model <= 255; model++ {
		body, head, ok := ModelAppearance(byte(model))
		valid := model >= 1 && model <= len(want)
		if ok != valid || valid && [2]byte{body, head} != want[model-1] {
			t.Fatalf("model %d: %d/%d valid=%v", model, body, head, ok)
		}
	}
}
