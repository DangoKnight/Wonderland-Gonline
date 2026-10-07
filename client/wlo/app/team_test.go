package app

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestTeamPetActionsAndAuthoritativeReplies(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	capture := wire(t, c)
	consumed := 0
	sent := func() [][]byte { packets := capture(); out := packets[consumed:]; consumed = len(packets); return out }
	c.InventoryState.Pets[2] = inventory.UsePet{ID: 12032, Name: []byte("Robinson"), Amity: 60, Stats: world.Stats{Level: 1, HP: 100, SP: 50, MaxHP: 100, MaxSP: 50, Element: 4}}
	c.MainButtons.Buttons[mainTeamButton].OnClick()
	if !c.Team.Visible || c.Inventory.Selected != 0 {
		t.Fatal("team visibility/independent inventory selection")
	}
	c.Team.SetMode(3, 1)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{19, 1, 0, 47, 0, 0}) {
		t.Fatal("battle pet request", got)
	}
	if c.TeamState.BattlePet != 0 {
		t.Fatal("optimistic battle pet")
	}
	c.dispatch([]byte{19, 1, 0, 47, 0, 0})
	if c.TeamState.BattlePet != 12032 {
		t.Fatal("battle reply")
	}
	c.Team.SetMode(3, 2)
	if got := sent(); len(got) != 2 || !bytes.Equal(got[0], []byte{19, 2}) || !bytes.Equal(got[1], []byte{15, 11, 3, 0, 47, 0, 0}) {
		t.Fatal("ride request", got)
	}
	c.dispatch([]byte{19, 2})
	mount := protocol.Builder{15, 16, 3}.U32(c.World.Player.ID).U32(12032).Bytes(make([]byte, 26))
	c.dispatch(mount)
	if c.TeamState.MountPet != 12032 || c.TeamState.BattlePet != 0 {
		t.Fatal("mount reply")
	}
	c.Team.SetMode(3, 0)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{15, 12, 3, 0, 47, 0, 0}) {
		t.Fatal("rest mount request", got)
	}
	c.dispatch(protocol.Builder{15, 17}.U32(c.World.Player.ID))
	if c.TeamState.MountPet != 0 {
		t.Fatal("dismount")
	}
	c.Team.DismissPet(3)
	if len(sent()) != 0 || c.UI.Modal == nil {
		t.Fatal("dismiss must confirm")
	}
	if path := os.Getenv("TEAM_DISMISS_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
	// Dismiss hides Teams as in the native client; reopen after canceling.
	if c.Team.Visible {
		t.Fatal("dismiss prompt must hide Teams")
	}
	// Cancel via Escape, then confirm through the modal's native OK control.
	c.UI.Modal.KeyDown(0x1b, 0)
	if c.UI.Modal != nil || len(sent()) != 0 {
		t.Fatal("cancel released pet")
	}
	c.Team.Show()
	c.Team.DismissPet(3)
	modal := c.UI.Modal
	for _, child := range modal.Base().Children {
		if child == modal.Base().Children[1] {
			child.(*seui.FixedButton).Click()
			break
		}
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{15, 2, 3}) {
		t.Fatal("release signature", got)
	}
	if c.InventoryState.Pets[2].ID != 12032 {
		t.Fatal("optimistic release")
	}
	c.dispatch(protocol.Builder{15, 2}.U32(c.World.Player.ID).U8(3))
	if c.InventoryState.Pets[2].ID != 0 || c.Inventory.Selected != 0 {
		t.Fatal("release roster/selection")
	}
}
func TestTeamRosterNativeStatsAndChat(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	peer := game.Character{ID: 20002, Slot: 1, Name: "DangoTest1", Body: 1, Head: 1, Level: 1, Element: 1, Map: c.World.Player.Map, X: 900, Y: 1100}
	p, err := peer.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	c.dispatch(protocol.Builder{13, 6}.U32(c.World.Player.ID).U8(1).U32(peer.ID))
	c.dispatch(protocol.Builder{8, 3}.U32(peer.ID).Bytes([]byte{35, 1, 1, 0, 0, 0, 0, 0, 0, 0}))
	c.dispatch(protocol.Builder{8, 3}.U32(peer.ID).Bytes([]byte{25, 1, 181, 0, 0, 0, 0, 0, 0, 0}))
	if !c.inTeam() || c.TeamState.Vitals[peer.ID].Stats.Level != 1 || c.TeamState.Vitals[peer.ID].Stats.HP != 181 {
		t.Fatal("teammate values")
	}
	c.Team.Show()
	c.Frame()
	if out := os.Getenv("TEAM_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	// Appearance metadata remains available when the teammate leaves this map.
	c.World.RemovePeer(peer.ID)
	if c.teamAppearances[peer.ID] == nil || c.Team.NameOf(peer.ID) == nil {
		t.Fatal("lost remote teammate")
	}
	c.dispatch(protocol.Builder{13, 6}.U32(c.World.Player.ID).U8(0))
	if c.inTeam() {
		t.Fatal("team chat membership retained")
	}
}
func TestTeamInvitationResponse(t *testing.T) {
	c, _, _ := enteredClient(t)
	chatLines := len(c.Chat.Lines)
	c.mapReady = true
	sent := wire(t, c)
	c.dispatch([]byte{13, 1, 34, 78, 0, 0, 3, 'A', 'n', 'n'})
	if c.UI.Modal == nil {
		t.Fatal("no invitation confirmation")
	}
	if path := os.Getenv("TEAM_INVITATION_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
	for _, child := range c.UI.Modal.Base().Children {
		if child.Base().Left == 70 {
			child.(*seui.FixedButton).Click()
			break
		}
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 3, 1, 34, 78, 0, 0}) {
		t.Fatal("invite acceptance", got)
	}
	if len(c.Chat.Lines) != chatLines {
		t.Fatal("incoming team request or acceptance added a chat notification")
	}
}

func TestTeamInvitationDecline(t *testing.T) {
	c, _, _ := enteredClient(t)
	chatLines := len(c.Chat.Lines)
	c.mapReady = true
	sent := wire(t, c)
	c.dispatch(protocol.Builder{13, 1}.U32(20002).U8(4).Bytes([]byte("Test")))
	if c.UI.Modal == nil {
		t.Fatal("no invitation")
	}
	c.UI.Modal.KeyDown(0x1b, 0)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{13, 3, 2, 34, 78, 0, 0}) {
		t.Fatal("decline reply", got)
	}
	if len(c.Chat.Lines) != chatLines {
		t.Fatal("incoming team request or decline added a chat notification")
	}
}
func TestInstanceBrowserAndCreation(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.Team.Instances.Show()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{85, 1, 1}) {
		t.Fatal("browse request", got)
	}
	f := c.Team.Instances
	if len(f.Definitions) != 34 {
		t.Fatal("native definitions missing", len(f.Definitions))
	}
	c.dispatch([]byte{85, 1, 0, 0, 0})
	f.ShowCreate()
	f.Selected = 0
	f.Refresh()
	if string(f.DefinitionPageLabel.Text) != "1/4" || string(f.Labels[0].Text) != f.Definitions[0].Name {
		t.Fatal("definition names or pagination missing")
	}
	f.Name.SetText([]byte("My Room"))
	f.Create()
	if got := sent(); len(got) != 2 || got[1][0] != 85 || got[1][1] != 3 {
		t.Fatal("create request", got)
	}
	packet := protocol.Builder{85, 203}.U16(61501).U16(f.Definitions[0].ID).U8(1).U32(c.World.Player.ID).U8(1)
	packet, _ = packet.String("Tester")
	c.dispatch(packet)
	if f.Room != 61501 || len(f.Members) != 1 || f.New.Visible || !f.Start.Visible {
		t.Fatal("room snapshot", f.Room, f.Members)
	}
	originalMap := c.World.Player.Map
	c.dispatch([]byte{85, 2, 11})
	if c.World.Player.Map != originalMap {
		t.Fatal("pending dungeon changed map")
	}
	if out := os.Getenv("INSTANCE_SNAPSHOT"); out != "" {
		f.Room = 0
		f.Members = nil
		f.Refresh()
		f.ShowCreate()
		f.Selected = 0
		f.Refresh()
		c.Frame()
		savePNG(t, out, c)
	}
	f.Reset()
	if f.Room != 0 || f.Visible || f.New.Visible {
		t.Fatal("instance state retained")
	}
}

func TestJoinTeamToolbarSelection(t *testing.T) {
	c, _, _ := enteredClient(t)
	chatLines := len(c.Chat.Lines)
	c.mapReady = true
	capture := wire(t, c)
	button := c.FuncButtons.Buttons[joinTeamButton]
	// Exercise the actual toolbar mouse path rather than invoking its callback.
	r := button.Base().Rect()
	c.Input.X, c.Input.Y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	c.Frame()
	if c.Input.Hovered != button {
		t.Fatalf("join button not reachable: %T", c.Input.Hovered)
	}
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if !c.joinTeamTarget || !button.IconDown {
		t.Fatal("join selection did not arm")
	}
	c.Input.Hovered = nil
	c.updateCursor(c.Now())
	if c.Cursors.Current != cursor.ShapePoint {
		t.Fatal("missing selection cursor")
	}
	c.GroundClick(10, 400)
	if !c.joinTeamTarget || len(capture()) != 0 || c.World.Walking() {
		t.Fatal("empty selection should wait without walking")
	}
	if !c.TeamKey(escapeKey, 0) || c.joinTeamTarget || button.IconDown {
		t.Fatal("escape did not cancel")
	}
	button.Click()
	button.Click()
	if c.joinTeamTarget || button.IconDown {
		t.Fatal("second click did not cancel")
	}
	peer := &world.Peer{Player: world.Player{ID: 20002, X: c.World.Player.X + 50, Y: c.World.Player.Y}}
	c.World.Peers = map[uint32]*world.Peer{peer.ID: peer}
	button.Click()
	cx, cy := c.World.Camera()
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	packets := capture()
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{13, 1, 34, 78, 0, 0}) {
		t.Fatalf("join request: %v", packets)
	}
	if c.joinTeamTarget || button.IconDown {
		t.Fatal("request did not finish selection")
	}
	if c.inTeam() {
		t.Fatal("membership changed before acceptance")
	}
	if len(c.Chat.Lines) != chatLines {
		t.Fatal("team selection or outgoing request added a chat notification")
	}
	c.dispatch(protocol.Builder{13, 6}.U32(peer.ID).U8(1).U32(c.World.Player.ID))
	if !c.inTeam() {
		t.Fatal("authoritative roster did not join")
	}
	button.Click()
	if c.joinTeamTarget || len(capture()) != 1 {
		t.Fatal("already joined character requested another party")
	}
	if len(c.Chat.Lines) != chatLines {
		t.Fatal("blocked team request added a chat notification")
	}
}

func TestTeamSharedWindowAndPetMenu(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Name: []byte("Robinson"), Amity: 60, Stats: world.Stats{Level: 1, HP: 100, MaxHP: 100, SP: 50, MaxSP: 50, Element: 4}}
	c.Team.Show()
	c.Team.Left, c.Team.Top = 80, 12
	c.Team.Instances.Show()
	if !c.Team.Visible || !c.Team.Instances.Visible || c.Team.Instances.Root() != c.Team {
		t.Fatal("instance tab must belong to the Teams window")
	}
	for _, form := range c.UI.Forms {
		if form == c.Team.Instances {
			t.Fatal("instance tab registered as another window")
		}
	}
	c.Team.MouseMove(0, 100, 24) // idle movement must not move the window
	c.Team.Dragging = true
	c.Team.DragX, c.Team.DragY = 10, 10
	c.Team.MouseMove(0, 150, 30)
	c.Team.Dragging = false
	position := c.Team.Abs()
	c.Team.SelectTab(false)
	if c.Team.Abs() != position || c.Team.Instances.Visible {
		t.Fatal("tabs lost shared position")
	}
	c.Team.Modes[0].Click()
	c.Frame()
	for _, option := range c.Team.Options {
		if !option.Visible || !option.HasHint {
			t.Fatal("missing pet action or tooltip")
		}
	}
	action := c.Team.Options[1].Abs()
	c.Input.X, c.Input.Y = action.X+5, action.Y+5
	c.Frame()
	if c.Input.Hovered != c.Team.Options[1] {
		t.Fatal("pet action does not receive hover")
	}
	c.UI.MouseDown(seui.ButtonRight, 0, 0, 400)
	for _, option := range c.Team.Options {
		if option.Visible {
			t.Fatal("outside click left pet menu open")
		}
	}
	// Portrait lookup must work even before any overworld NPC initialized templates.
	c.npcTemplates = nil
	r := image.Rect(0, 0, 24, 24)
	c.Screen.Fill(r, 0)
	c.Team.DrawPetFace(1, r)
	drawn := false
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if c.Screen.Pix[y*c.Screen.W+x] != 0 {
				drawn = true
			}
		}
	}
	if !drawn {
		t.Fatal("Robinson portrait missing before NPC templates load")
	}
	if path := os.Getenv("TEAM_CONTROLS_SNAPSHOT"); path != "" {
		c.Team.Modes[0].Click()
		c.Frame()
		savePNG(t, path, c)
	}
}

func TestTeamPetPortraitCompiledAssets(t *testing.T) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE to compiled client assets")
	}
	c, _, _ := enteredClient(t)
	useCompiledPetAssets(t, c, bundle)
	c.npcTemplates = nil
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032}
	r := image.Rect(0, 0, 24, 24)
	c.Screen.Fill(r, 0)
	c.Team.DrawPetFace(1, r)
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if c.Screen.Pix[y*c.Screen.W+x] != 0 {
				return
			}
		}
	}
	t.Fatal("compiled Robinson portrait is empty")
}

func useCompiledPetAssets(t *testing.T, c *Client, bundle string) {
	t.Helper()
	root, closeBundle, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeBundle() })
	c.Assets = login.NewAssets(root)
	spriteRoot := filepath.Join(root, "sprites")
	c.lib = role.NewLibraryWith(sprites.NewManager([]string{spriteRoot}, []string{spriteRoot}))
	t.Cleanup(func() { _ = c.lib.Sprites.Close() })
	c.resources.library = c.lib
	c.resources.petPortraits = nil
	c.npcTemplates = nil
}

func TestTeamNativeSmallPortraitAndPersistentTabs(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032}
	c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	robinson := c.npcTemplates[12032]
	if robinson.Icon != 7143 || robinson.Face != 8067 {
		t.Fatalf("native portrait references: %+v", robinson)
	}
	// A small icon must work without any large dialogue portrait.
	robinson.Face = 0
	c.npcTemplates = map[uint32]world.NPCTemplate{12032: robinson}
	c.resources.petPortraits = nil
	c.Team.Show()
	for _, instances := range []bool{false, true, false, true, false} {
		c.Team.SelectTab(instances)
		for i := 0; i < 3; i++ {
			c.Frame()
		}
		for index, tab := range c.Team.Tabs {
			selected := (index == 1) == instances
			if tab.Sticky != selected || (selected && tab.State != 2) {
				t.Fatalf("tab %d instances=%t state=%d sticky=%t", index, instances, tab.State, tab.Sticky)
			}
		}
	}
	portrait := c.resources.petPortrait(robinson)
	if portrait == nil || portrait.W != 24 || portrait.H != 24 {
		t.Fatal("dedicated 24x24 portrait missing")
	}
	// Compare the actual form's portrait region, after all child controls paint.
	at := c.Team.Abs().Add(image.Pt(29, 74))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if want := portrait.Pix[y*24+x]; want != 0 && c.Screen.Pix[(at.Y+y)*c.Screen.W+at.X+x] != want {
				t.Fatalf("pet icon overwritten at %d,%d", x, y)
			}
		}
	}
}

func TestTeamInlinePetRenameAndNoRowSelection(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	capture := wire(t, c)
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Name: []byte("Robinson")}
	c.Team.Show()
	click := func(x, y int) {
		c.Input.X, c.Input.Y = x, y
		c.Frame()
		c.UI.MouseDown(seui.ButtonLeft, 0, x, y)
		c.UI.MouseUp(seui.ButtonLeft, 0, x, y)
		c.Frame()
	}
	a := c.Team.Abs()
	click(a.X+40, a.Y+90)
	if c.Inventory.Selected != 0 || len(capture()) != 0 {
		t.Fatal("portrait click selected a pet or sent a command")
	}
	e := c.Team.Names[0]
	r := e.Rect()
	click(r.Min.X+8, r.Min.Y+8)
	if e.ReadOnly || c.Input.Focused != e {
		t.Fatal("name click did not start inline editing")
	}
	if e.Clip != image.Rect(0, 0, 83, 13) || e.Height != 23 || e.Width != 86 {
		t.Fatal("rename panel must use native source clip and editor size", e.Clip, e.Width, e.Height)
	}
	if path := os.Getenv("TEAM_RENAME_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
	e.SetText([]byte("Friday"))
	c.UI.KeyDown(seui.VKReturn, 0)
	packets := capture()
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{15, 6, 1, 'F', 'r', 'i', 'd', 'a', 'y'}) {
		t.Fatal("rename must send raw name after slot", packets)
	}
	if string(c.InventoryState.Pets[0].Name) != "Robinson" || !e.ReadOnly {
		t.Fatal("rename was optimistic or editing did not end")
	}
	c.dispatch(protocol.Builder{15, 9}.U32(c.World.Player.ID).U8(1).Bytes([]byte("Friday")))
	c.Frame()
	if string(e.Text) != "Friday" || e.Caret != 0 || e.CaretCol != 0 {
		t.Fatal("authoritative name not displayed", string(e.Text), e.Caret, e.CaretCol)
	}
	click(r.Min.X+8, r.Min.Y+8)
	e.SetText([]byte("Monday"))
	click(a.X+40, a.Y+90)
	if packets = capture(); len(packets) != 2 || !bytes.Equal(packets[1], []byte{15, 6, 1, 'M', 'o', 'n', 'd', 'a', 'y'}) {
		t.Fatal("blur did not submit exactly once", packets)
	}
	// A pet replaced during editing must not inherit the old pet's rename.
	click(r.Min.X+8, r.Min.Y+8)
	e.SetText([]byte("WrongPet"))
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 10001, Name: []byte("NewPet")}
	c.Frame()
	if len(capture()) != 2 || !e.ReadOnly || string(e.Text) != "NewPet" {
		t.Fatal("rename leaked into a replacement slot")
	}
	click(r.Min.X+8, r.Min.Y+8)
	e.SetText(nil)
	c.Team.Hide()
	if len(capture()) != 2 || !e.ReadOnly {
		t.Fatal("empty rename or hide left editor active")
	}
}

func TestTeamFollowingPetNickname(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Name: []byte("Friday")}
	c.showCompanion(c.World.Player.ID, 12032, nil)
	pet := c.World.Companions[c.World.Player.ID]
	if pet == nil || string(pet.Name) != "Friday" {
		t.Fatal("following pet ignored roster nickname", pet)
	}
	c.dispatch(protocol.Builder{15, 9}.U32(c.World.Player.ID).U8(1).Bytes([]byte("Monday")))
	if string(pet.Name) != "Monday" {
		t.Fatal("rename reply did not refresh following name", string(pet.Name))
	}
	c.showCompanion(c.World.Player.ID, 12032, nil)
	if string(pet.Name) != "Monday" {
		t.Fatal("mode refresh reverted nickname")
	}
}

func TestTeamDismissMatchesLogoutBackground(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Team.Confirm("Leave the party?", 0, func() {})
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	c.UI.Modal.Paint()
	want := append([]uint16(nil), c.Screen.Pix...)
	c.UI.Modal.KeyDown(seui.VKEscape, 0)
	c.logoutConfirmation(false)
	c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
	c.logoutPrompt.Paint()
	if !slices.Equal(want, c.Screen.Pix) {
		t.Fatal("Teams confirmation background differs from logout")
	}
	c.closeSettingsPrompt()
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Name: []byte("Friday")}
	var at image.Point
	var action int
	c.Team.DrawPetBody = func(_ byte, x, y, pose int) { at = image.Pt(x, y); action = pose }
	c.Team.Show()
	c.Team.DismissPet(1)
	c.UI.Modal.Paint()
	origin := c.UI.Modal.Base().Abs()
	w, h := c.Team.PetPreviewSize(1)
	if at != origin.Add(image.Pt(c.UI.Modal.Base().Width-w/2-30, h+20)) || action != 11 {
		t.Fatal("dismiss portrait must use native right-side body placement and pose", at, origin, action)
	}
}

func TestTeamPetConfirmationHeightAndStaticPose(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	previousHeight := 0
	for _, scenario := range []struct {
		id   uint16
		name string
	}{{12032, "Robinson"}, {17162, "S.Monkey"}} {
		c.InventoryState.Pets[0] = inventory.UsePet{ID: scenario.id, Name: []byte(scenario.name)}
		c.Team.Show()
		c.Team.DismissPet(1)
		f := c.UI.Modal.Base()
		w, h := c.Team.PetPreviewSize(1)
		if w == 0 || h == 0 {
			t.Fatal("preview sprite missing", scenario.id)
		}
		if scenario.id == 17162 && f.Height >= previousHeight {
			t.Fatal("short pet must have shorter confirmation", f.Height, previousHeight)
		}
		previousHeight = f.Height
		if f.Height != max(81, h+70)+40 {
			t.Fatal("native body-height sizing", f.Height, h)
		}
		c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
		c.Team.DrawPetBody(1, 300, 200, 11)
		before := append([]uint16(nil), c.Screen.Pix...)
		*now = now.Add(time.Second)
		c.Screen.Fill(image.Rect(0, 0, 800, 600), 0)
		c.Team.DrawPetBody(1, 300, 200, 11)
		if !slices.Equal(before, c.Screen.Pix) {
			t.Fatal("pet confirmation preview animated", scenario.id)
		}
		c.UI.Modal.KeyDown(seui.VKEscape, 0)
	}
}

func TestLogoutConfirmationStaticNativePose(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	c.logoutConfirmation(false)
	f := c.logoutPrompt
	if f.preview == c.Inventory.Preview {
		t.Fatal("logout preview must have independent animation state")
	}
	paint := func() { c.Screen.Fill(image.Rect(0, 0, 800, 600), 0); f.Update(c.Input) }
	paint()
	before := append([]uint16(nil), c.Screen.Pix...)
	*now = now.Add(time.Second)
	paint()
	if !slices.Equal(before, c.Screen.Pix) {
		t.Fatal("logout body animated")
	}
	if logoutPreviewAction != 13 {
		t.Fatal("native human confirmation pose")
	}
}
