package ui

import (
	"image"
	"testing"
)

func click(x, y int) []*Input {
	return []*Input{{X: x, Y: y, Down: true, Pressed: true}, {X: x, Y: y, Released: true}}
}

func TestServerSelectFlow(t *testing.T) {
	a := testAssets(t)
	ini := "01[Rhodes Island]1\r\nRhodes1*10.0.0.1\r\nRhodes2<R>*10.0.0.2\r\n02[Lion Island]<G>2\r\nLion1*10.0.0.3\r\n"
	regions, _ := ParseServerINI([]byte(ini))
	s := NewServerSelect(regions)
	var region Region
	var chosen []Server
	s.OnRegion = func(r Region) { region = r }
	s.OnChoose = func(sv Server) { chosen = append(chosen, sv) }
	run := func(ins []*Input) {
		for _, in := range ins {
			s.Update(a, in)
		}
	}
	// Next with nothing selected does nothing.
	run(click(330, 450))
	if len(chosen) != 0 {
		t.Fatal(chosen)
	}
	run(click(300, 215)) // first region row
	if region.Name != "Rhodes Island" || len(s.serverList.items) != 2 {
		t.Fatal(region, s.serverList.items)
	}
	if it := s.serverList.items[1]; string(it.text) != "Rhodes2" || it.color != 0xf800 {
		t.Fatalf("%q %#x", it.text, it.color)
	}
	run(click(430, 235)) // second server row
	run(click(330, 450)) // Next
	if len(chosen) != 1 || chosen[0].Address != "10.0.0.2" || chosen[0].ID != 102 {
		t.Fatal(chosen)
	}
	run(click(300, 235)) // second region
	if region.ID != 2 || region.Color != 0x0540 || s.serverList.selected != -1 {
		t.Fatal(region, s.serverList.selected)
	}
	// A double click on a server chooses it.
	run([]*Input{{X: 430, Y: 215, Pressed: true, DoubleClick: true}})
	if len(chosen) != 2 || chosen[1].Name != "Lion1" {
		t.Fatal(chosen)
	}
}

func TestButtonStates(t *testing.T) {
	a := testAssets(t)
	clicks := 0
	b := &button{sprite: "Btn_Login_L.bmp", at: image.Pt(10, 10), onClick: func() { clicks++ }}
	steps := []struct {
		in    Input
		frame int
	}{
		{Input{X: 20, Y: 15}, 1},
		{Input{X: 20, Y: 15, Down: true, Pressed: true}, 2},
		{Input{X: 20, Y: 15, Down: true}, 2},
		{Input{X: 20, Y: 15, Released: true}, 1},
		{Input{X: 20, Y: 15, Down: true, Pressed: true}, 2},
		{Input{X: 300, Y: 15, Down: true}, 0}, // leaving cancels the press
		{Input{X: 20, Y: 15, Released: true}, 1},
	}
	for i, st := range steps {
		b.update(a, &st.in)
		if b.frame != st.frame {
			t.Fatalf("step %d: frame %d, want %d", i, b.frame, st.frame)
		}
	}
	if clicks != 1 {
		t.Fatal("clicks", clicks)
	}
}

func TestLoginFlow(t *testing.T) {
	a := testAssets(t)
	l := NewLogin()
	var got []string
	l.OnLogin = func(account, password string) { got = append(got, account, password) }
	prev := 0
	l.OnPrevious = func() { prev++ }
	l.Update(a, &Input{Text: []byte("tester"), Keys: []Key{KeyEnter}})
	l.Update(a, &Input{Keys: []Key{KeyEnter}}) // empty password
	if len(got) != 0 || l.message == "" {
		t.Fatal(got, l.message)
	}
	l.Update(a, &Input{Text: []byte("tester1"), Keys: []Key{KeyEnter}})
	if len(got) != 2 || got[0] != "tester" || got[1] != "tester1" {
		t.Fatal(got)
	}
	// Ten characters at most.
	l.Reset()
	l.SetAccount("")
	l.Update(a, &Input{Text: []byte("abcdefghijklmnop")})
	if l.Account() != "abcdefghij" {
		t.Fatal(l.Account())
	}
	for _, in := range click(400, 375) { // Previous
		l.Update(a, in)
	}
	if prev != 1 {
		t.Fatal("previous", prev)
	}
	for _, in := range click(335, 312) { // remember box
		l.Update(a, in)
	}
	if !l.remember || l.rememberBox.sprite != "btn_Check_1.bmp" {
		t.Fatal(l.remember)
	}
}

func TestParseServerINI(t *testing.T) {
	regions, hidden := ParseServerINI([]byte("02[B]2\n\n01[A]1\r\nA1*1.1.1.1\r\nNoAddr\r\n*2.2.2.2\r\n95[Hidden]9\r\nH*3.3.3.3\r\n03[Closed]\r\n"))
	if len(regions) != 3 || regions[0].Name != "A" || regions[1].Name != "B" || regions[2].ID != 0 {
		t.Fatal(regions)
	}
	if s := regions[0].Servers; len(s) != 2 || s[1].Name != "No Name" || s[1].ID != 102 {
		t.Fatal(s)
	}
	if hidden == nil || hidden.Servers[0].Address != "3.3.3.3" {
		t.Fatal(hidden)
	}
}
