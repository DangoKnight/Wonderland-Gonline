package app

import (
	"image/png"
	"net"
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestWorkspaceSharedAssetsAndIndependentSessions(t *testing.T) {
	c, _, _ := enteredClient(t)
	w := NewWorkspace(c)
	w.factory = func(o Options) (*Client, error) {
		next, err := New(o)
		if err == nil {
			next.Net.Dial = c.Net.Dial
		}
		return next, err
	}
	defer w.Close()
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 3}
	c.SkillState.Learned[11016] = skills.Progress{Grade: 1}
	c.HotKeys.Bindings[1][1] = hud.Binding{Kind: 2, ID: 10001}
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	next := w.Current()
	if next == c || next.Net == c.Net || next.UI == c.UI || next.Input == c.Input || next.G == c.G || next.Screen == c.Screen {
		t.Fatal("mutable session resources shared")
	}
	if next.Pics != c.Pics || next.Sprites != c.Sprites || next.Env.Text.Font != c.Env.Text.Font || next.SkillState.Catalog != c.SkillState.Catalog || next.background != c.background || next.lib != c.lib {
		t.Fatal("decoded asset caches duplicated")
	}
	if next.Cursors == c.Cursors || next.Cursors.Shapes[12] != c.Cursors.Shapes[12] {
		t.Fatal("cursor playback not isolated from shared artwork")
	}
	if next.InventoryState.Bag[0].ID != 0 || next.SkillState.Learned[11016].Grade != 0 || next.HotKeys.Bindings[1][1].ID != 0 {
		t.Fatal("player state leaked across sessions")
	}
	if next.settingsPath == c.settingsPath || next.Assets.UserPath("save.dat") == c.Assets.UserPath("save.dat") {
		t.Fatal("writable profiles shared")
	}
	if !c.backgroundSession || next.backgroundSession {
		t.Fatal("audio focus wrong")
	}
	c.Input.Pressed = c.HotKeys.Slots[1]
	w.Switch(0)
	if c.Input.Pressed != nil || c.backgroundSession || !next.backgroundSession {
		t.Fatal("switch retained gesture or audio focus")
	}
	c.InventoryState.Bag[0].Count = 2
	if next.InventoryState.Bag[0].Count != 0 {
		t.Fatal("inventory mutation leaked")
	}
	next.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 9}
	next.SkillState.Learned[11016] = skills.Progress{Grade: 2}
	if c.SkillState.Learned[11016].Grade != 1 {
		t.Fatal("skill mutation leaked")
	}
	w.Remove(2)
	if w.Current() != c || len(w.Sessions) != 1 || c.Sprites == nil {
		t.Fatal("removing another session destroyed shared resources")
	}
	w.Tick()
	w.Remove(1)
	if w.Current() != nil {
		t.Fatal("last session retained")
	}
}
func TestWorkspacePanelConsumesInputAndRenders(t *testing.T) {
	c := testClient(t)
	w := NewWorkspace(c)
	defer w.Close()
	w.factory = func(o Options) (*Client, error) {
		next, err := New(o)
		if err == nil {
			next.Net.Dial = c.Net.Dial
		}
		return next, err
	}
	c.Frame()
	x, y := ScreenWidth+5, 5
	if !w.Pointer(x, y, true, true, 0) {
		t.Fatal("permanent panel did not consume input")
	}
	if !w.Pointer(200, 200, false, false, 0) {
		t.Fatal("panel press leaked on release")
	}
	if w.Pointer(200, 200, false, false, 0) {
		t.Fatal("map remained captured")
	}
	add := w.addRect()
	w.Pointer(add.Min.X+40, add.Min.Y+40, true, true, 0)
	if len(w.Sessions) != 2 || w.Active != 1 {
		t.Fatal("add button failed")
	}
	w.Pointer(add.Min.X+40, add.Min.Y+40, false, false, 0)
	first := w.cardRect(0)
	w.Pointer(first.Min.X+40, first.Min.Y+25, true, true, 0)
	if w.Active != 0 {
		t.Fatal("thumbnail did not select session")
	}
	w.Pointer(first.Min.X+40, first.Min.Y+25, false, false, 0)
	w.Pointer(workspaceWidth-sessionPanelPadding-12, sessionListTop+sessionRowHeight+12, true, true, 0)
	if w.PendingRemove != 2 || len(w.Sessions) != 2 {
		t.Fatal("remove did not require confirmation")
	}
	cancel := w.confirmRect(1, false)
	w.Pointer(cancel.Min.X+5, cancel.Min.Y+5, true, true, 0)
	if w.PendingRemove != 0 || len(w.Sessions) != 2 {
		t.Fatal("cancel removed session")
	}
	w.Pointer(workspaceWidth-sessionPanelPadding-12, sessionListTop+sessionRowHeight+12, true, true, 0)
	confirm := w.confirmRect(1, true)
	w.Pointer(confirm.Min.X+5, confirm.Min.Y+5, true, true, 0)
	if len(w.Sessions) != 1 {
		t.Fatal("confirmed removal failed")
	}
	before := append([]uint16(nil), c.Screen.Pix...)
	view := w.Compose()
	if view == c.Screen {
		t.Fatal("sidebar drawn into the session preview")
	}
	if view.W != ScreenWidth+220 || view.H != ScreenHeight {
		t.Fatal("workspace did not reserve extra width")
	}
	for y := 0; y < ScreenHeight; y++ {
		for x := 0; x < ScreenWidth; x++ {
			i := y*ScreenWidth + x
			if c.Screen.Pix[i] != before[i] || view.Pix[y*view.W+x] != before[i] {
				t.Fatalf("sidebar changed game viewport at %d,%d", x, y)
			}
		}
	}
	if w.SidebarContains(ScreenWidth-1, 20) || !w.SidebarContains(ScreenWidth, 20) || w.SidebarContains(workspaceWidth, 20) || w.SidebarContains(ScreenWidth, -1) {
		t.Fatal("sidebar input boundary overlaps or exceeds workspace")
	}
	w.Pointer(ScreenWidth+10, 20, false, false, 0) // Release the confirmation click.
	if w.Pointer(ScreenWidth-1, 20, true, false, 0) {
		t.Fatal("right edge of game consumed by sidebar")
	}
	if path := os.Getenv("WORKSPACE_SNAPSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, view.RGBA()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

func TestWorkspaceBackgroundNetworkWithoutRedraw(t *testing.T) {
	c, _, _ := enteredClient(t)
	w := NewWorkspace(c)
	defer w.Close()
	w.factory = func(o Options) (*Client, error) {
		next, err := New(o)
		if err == nil {
			next.Net.Dial = c.Net.Dial
		}
		return next, err
	}
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	near, far := net.Pipe()
	defer far.Close()
	c.Net.Dial = func(string, string) (net.Conn, error) { return near, nil }
	c.Net.Connect("localhost")
	connected := false
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && !connected; {
		for _, event := range c.Net.Poll() {
			connected = connected || event.Kind == login.Connected
		}
		if !connected {
			time.Sleep(time.Millisecond)
		}
	}
	if !connected {
		t.Fatal("test connection unavailable")
	}
	before := append([]uint16(nil), c.Screen.Pix...)
	// Native AC26:4 changes gold on the inactive character, with no UI input.
	if err := protocol.Write(far, []byte{26, 4, 123, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && c.Stats.Gold != 123; {
		w.Tick()
		time.Sleep(time.Millisecond)
	}
	if c.Stats.Gold != 123 || w.Current() == c {
		t.Fatal("inactive session did not process its own server packet")
	}
	if w.ticks >= sessionBackgroundFrameTicks {
		t.Fatal("test did not observe the no-redraw interval")
	}
	for i, v := range before {
		if c.Screen.Pix[i] != v {
			t.Fatal("background pixels redrawn before preview interval")
		}
	}
}

func TestWorkspaceFourCardsAndAddCardVisible(t *testing.T) {
	c, _, _ := enteredClient(t)
	w := NewWorkspace(c)
	defer w.Close()
	w.factory = func(o Options) (*Client, error) {
		next, err := New(o)
		if err == nil {
			next.Net.Dial = c.Net.Dial
		}
		return next, err
	}
	for len(w.Sessions) < 4 {
		if err := w.Add(); err != nil {
			t.Fatal(err)
		}
	}
	if w.visibleRows() < 4 || w.Scroll != 0 {
		t.Fatal("four sessions do not fit without scrolling")
	}
	for row := 0; row < 4; row++ {
		r := w.cardRect(row)
		if r.Min.Y < 0 || r.Max.Y > ScreenHeight || r.Min.X < ScreenWidth {
			t.Fatal("card outside side panel")
		}
		if !r.Intersect(w.addRect()).Empty() {
			t.Fatal("plus card overlaps visible session")
		}
		w.Pointer(r.Min.X+30, r.Min.Y+30, true, true, 0)
		w.Pointer(r.Min.X+30, r.Min.Y+30, false, false, 0)
		if w.Active != row {
			t.Fatalf("visible session %d not selectable", row)
		}
	}
	if r := w.addRect(); r.Max.Y > ScreenHeight || r.Min.Y < 0 {
		t.Fatal("plus card not fully visible")
	}
	// Gaps are inert; the add card creates a fifth instance and scrolls it into view.
	w.Pointer(ScreenWidth+20, sessionListTop+sessionCardHeight+2, true, false, 0)
	if w.Active != 3 || len(w.Sessions) != 4 {
		t.Fatal("card gap activated a control")
	}
	add := w.addRect()
	w.Pointer(add.Min.X+50, add.Min.Y+50, true, true, 0)
	w.Pointer(add.Min.X+50, add.Min.Y+50, false, false, 0)
	if len(w.Sessions) != 5 || w.Active != 4 || w.Scroll != 0 {
		t.Fatal("fifth session was not brought into view")
	}
	if w.addVisible() {
		t.Fatal("plus card pinned while fifth session visible")
	}
	w.Pointer(ScreenWidth+50, 60, false, false, -1)
	if w.Scroll != 1 || !w.addVisible() {
		t.Fatal("plus card did not scroll into view")
	}
	w.Pointer(ScreenWidth+50, 60, false, false, 1)
	if w.Scroll != 0 {
		t.Fatal("older sessions not accessible by scrolling")
	}
	w.Switch(0)
	c.Sprites.WaitNative()
	c.Frame()
	if path := os.Getenv("WORKSPACE_FOUR_SNAPSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, w.Compose().RGBA())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceCollapseCentersGameAndTranslatesInput(t *testing.T) {
	c, _, _ := enteredClient(t)
	w := NewWorkspace(c)
	defer w.Close()
	c.Frame()
	before := append([]uint16(nil), c.Screen.Pix...)
	toggle := w.toggleRect()
	x, y := toggle.Min.X+5, toggle.Min.Y+5
	w.PendingRemove = 1
	if !w.Pointer(x, y, true, true, 0) || !w.Collapsed || w.PendingRemove != 0 {
		t.Fatal("collapse control failed")
	}
	w.Pointer(x, y, false, false, 0)
	for step := 0; step < sessionSlideTicks; step++ {
		if _, _, inside := w.GamePoint(300, 300); inside {
			t.Fatal("game pointer enabled during slide")
		}
		w.stepLayout()
		view := w.Compose()
		left := w.gameLeft()
		if w.panelLeft() < left+ScreenWidth {
			t.Fatal("sliding panel covered game")
		}
		for row := 0; row < ScreenHeight; row++ {
			for col := 0; col < ScreenWidth; col++ {
				if view.Pix[row*view.W+left+col] != before[row*ScreenWidth+col] {
					t.Fatal("slide changed game viewport")
				}
			}
		}
	}
	if path := os.Getenv("WORKSPACE_COLLAPSED_SNAPSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, w.Compose().RGBA())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if w.gameLeft() != 110 || w.panelLeft() != 1020 || w.animating() {
		t.Fatal("collapsed game not centered")
	}
	gx, gy, inside := w.GamePoint(238, 260)
	if !inside || gx != 128 || gy != 260 {
		t.Fatal("centered game pointer not translated")
	}
	if w.Pointer(238, 260, false, false, 0) {
		t.Fatal("centered game input consumed")
	}
	for _, point := range [][2]int{{109, 260}, {910, 260}, {238, -1}, {238, 600}} {
		if _, _, inside := w.GamePoint(point[0], point[1]); inside {
			t.Fatal("margin treated as game")
		}
	}
	if !w.Pointer(x, y, true, true, 0) || w.Collapsed {
		t.Fatal("expand button failed")
	}
	w.Pointer(x, y, false, false, 0)
	for step := 0; step < sessionSlideTicks; step++ {
		w.stepLayout()
	}
	if w.gameLeft() != 0 || w.panelLeft() != 800 || w.animating() {
		t.Fatal("expanded layout not restored")
	}
	if _, _, inside := w.GamePoint(128, 260); !inside {
		t.Fatal("expanded game input not restored")
	}
	// Reversing direction mid-slide must return smoothly to its starting layout.
	w.Collapsed = true
	w.stepLayout()
	w.stepLayout()
	w.stepLayout()
	previous := w.gameLeft()
	w.Collapsed = false
	w.stepLayout()
	if w.gameLeft() > previous {
		t.Fatal("reversed slide continued in old direction")
	}
}
