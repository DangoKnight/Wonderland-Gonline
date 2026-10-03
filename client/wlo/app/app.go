// Package app is the client's main form (TForm1): startup (FormCreate,
// 0x493fb0), the frame (DXTimer1Timer, 0x4a1d60) and the window's input
// handlers, running the login phase.
package app

import (
	"encoding/binary"
	"image"
	"os"
	"path/filepath"
	"time"

	"wonderland-go/client/wlo/cursor"
	"wonderland-go/client/wlo/hud"
	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/role"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/sprites"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/client/wlo/text"
	"wonderland-go/client/wlo/world"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/clientassets"
	"wonderland-go/internal/protocol"
)

// Display and timing constants.
const (
	ScreenWidth  = 800
	ScreenHeight = 600
	// skinColor is the white skin's colour in Skins.Flst (2113).
	skinColor = 0x841
	// loginTimeout is _DAT_004a2e3c, measured from the login send.
	loginTimeout = 30000 * time.Millisecond
	// timeoutNotice is the duration given with "Can't reach login server".
	timeoutNotice = 2000 * time.Millisecond
	// wrongPasswordNotice is the duration of "Password wrong" (1/6).
	wrongPasswordNotice = 2000 * time.Millisecond
)

var (
	noticeTimeout       = []byte("Can't reach login server") // 0x4a2e48
	noticeWrongPassword = []byte("Password wrong")           // 0x2f02c8
	noticeConnection    = []byte("Connection lost")          // 0x4988d4
)

// Client is the running login phase.
type Client struct {
	Root   string
	Assets login.Assets

	Screen  *surface.Surface
	Pics    *picdb.DB
	Cursors *cursor.Cursors
	Env     *seui.Env
	UI      *seui.Manager
	Input   *seui.Input
	G       *login.Globals
	Net     *login.Net
	Servers *login.SelectServer
	Login   *login.IDPassword
	Chars   *login.SelectCharacter
	// Create is character creation (PTR_DAT_004ca398) and Password its
	// password dialog (DAT_006c6f40).
	Create   *login.CreateCharacter
	Password *login.InputPassword
	Sprites  *sprites.Manager
	Notices  Notices
	// World is the in-game view, set by AC3.
	World        *world.World
	Stats        *world.Stats     // the player's values (5/3, 8/1, 26/4)
	MainStatus   *hud.MainStatus  // the status panel over the world
	FuncButtons  *hud.FuncBtnForm // the toolbar at the top right
	MainButtons  *hud.MainBtnForm // the menu at the bottom right
	HotKeys      *hud.HotKeyForm  // the F1–F8 bar
	ChatBar      *hud.InputBar    // the chat bar
	groundHeld   bool             // the left button went down on the ground
	groundSince  time.Time        // when it went down
	uiHovered    bool             // a control was under the pointer last frame
	nextWalk     time.Time        // next re-plan while a button or arrow is held
	Chat         *hud.ChatLog     // the chat log over the map
	npcTemplates map[uint32]world.NPCTemplate
	Talk         *hud.Talk // the event talk window
	Music        *Music    // background music, nil without audio
	movie        *moviePlay
	sceneMusic   map[uint16]string
	talks        map[uint16]string
	event        eventState
	pendingNPC   *world.NPC // clicked out of reach, sent on arrival
	held         bool       // 6/2: the server holds the player

	lib   *role.Library
	items map[uint16]assets.NativeItem
	// Scene names (SceneData.dat) and each map's scene (eve.Emg), read on
	// first entry.
	sceneNames map[uint16]string
	mapScenes  map[uint16]uint16

	background *surface.Surface // login background object (0x40c27c)
	logo       int              // its Icon_LoginLogo_1
	// disconnectText is DAT_00828128 +0x18, left by an action-0 packet.
	disconnectText []byte
	// Now is the frame clock; tests replace it.
	Now func() time.Time
	// Exit is set by the Leave button.
	Exit bool
	// Unhandled receives packets the login phase does not handle.
	Unhandled func(p []byte)
}

// Options configure New.
type Options struct {
	// SpritesRoot is an optional directory of sprite packs or of the
	// editable export, searched before the decompiled data root's sprites/.
	SpritesRoot string
	Root        string // the asset directory (login.Assets)
	ServerINI   string // SERVER.INI override
	FallbackINI []byte // used when SERVER.INI cannot be read
}

// New is the login part of FormCreate: the picture database with the
// default and white skins, the font, the form manager in the white skin's
// colour, the login background and the two login forms.
func New(o Options) (*Client, error) {
	c := &Client{Root: o.Root, Assets: login.NewAssets(o.Root), Now: time.Now}
	a := c.Assets
	// The original install ships user\ for save.dat and AccountList.dat.
	os.MkdirAll(filepath.Dir(c.Assets.UserPath("save.dat")), 0o755)
	c.Screen = surface.New(ScreenWidth, ScreenHeight)
	c.Pics = picdb.New()
	// Direct data roots use standard PNG skins and bitmap font atlases.
	c.Pics.LoadDir(a.MediaPath("menu", "skins", "default"), false, 0)
	c.Pics.LoadDir(a.MediaPath("menu", "Skins", "white"), true, 0)
	font, err := clientassets.LoadFontAtlas(a.MediaPath("font", "TATPC1_TWN"))
	if err != nil {
		return nil, err
	}
	// Screen.Cursors from cursor\*.ani (0x3bb990), as exported frames.
	c.Cursors, err = cursor.LoadAll(a.MediaPath("cursor"), c.Now())
	if err != nil {
		return nil, err
	}
	// The ground shadows of pic\images.BMg (1.bls), from its PNG export.
	for _, name := range []string{world.ShadowPicture, world.MonsterShadowPicture} {
		m, err := a.LoadPicture(shadowArchive, name)
		if err != nil {
			return nil, err
		}
		c.Pics.Add(name, m)
	}
	c.Input = &seui.Input{}
	c.Env = &seui.Env{Pics: c.Pics, Screen: c.Screen, Text: &text.Renderer{Font: font}}
	c.UI = seui.NewManager(c.Env, c.Input)
	c.UI.SetColor(skinColor)

	// LogPic1.jpg, from its PNG export (decoded with the client's JPEG
	// rules).
	if m, err := a.LoadPicture("LogPic1"); err == nil {
		c.background = surface.FromImage(m)
	}
	c.logo = -1

	c.G = login.NewGlobals()
	c.Net = login.NewNet()
	c.Servers = login.NewSelectServer(c.Env, c.G, c.Net, a)
	c.Servers.INI, c.Servers.FallbackINI = o.ServerINI, o.FallbackINI
	c.Servers.OnExit = func() { c.Exit = true }
	c.Servers.OnConnecting = func() { c.Net.Send(login.Discovery()) }
	c.UI.Add(c.Servers)
	c.Login = login.NewIDPassword(c.Env, c.G, c.Net, a)
	c.Login.Servers = c.Servers
	c.Login.Notify = func(text []byte, d time.Duration) { c.Notices.Show(text, d, c.Now()) }
	c.UI.Add(c.Login)
	formula, err := login.LoadFormula(a)
	if err != nil {
		return nil, err
	}
	c.Sprites = sprites.NewManager(
		[]string{o.SpritesRoot},
		[]string{o.SpritesRoot, filepath.Join(a.Data, "sprites")})
	var jma001 int64
	if a, err := c.Sprites.Archive("001"); err == nil {
		jma001 = a.SourceBytes
	}
	content := login.ContentLevel(jma001)
	c.Chars = login.NewSelectCharacter(c.Env, c.G, c.Net, a, login.NewStatus(c.Env, formula), content)
	c.Stats = &world.Stats{Formula: formula}
	c.MainStatus = hud.NewMainStatus(c.Env, formula, c.Stats)
	c.UI.Add(c.MainStatus)
	c.FuncButtons = hud.NewFuncBtnForm(c.Env)
	c.UI.Add(c.FuncButtons)
	c.MainButtons = hud.NewMainBtnForm(c.Env)
	c.UI.Add(c.MainButtons)
	c.HotKeys = hud.NewHotKeyForm(c.Env, ScreenWidth, ScreenHeight)
	c.UI.Add(c.HotKeys)
	c.ChatBar = hud.NewInputBar(c.Env)
	c.UI.Add(c.ChatBar)
	c.ChatBar.Message.OnEnter = c.sendChat
	c.Chat = hud.NewChatLog(c.Env)
	c.Chat.Now = func() time.Time { return c.Now() }
	c.UI.Add(c.Chat)
	c.Talk = hud.NewTalk(c.Env)
	c.Talk.Now = func() time.Time { return c.Now() }
	c.Talk.Pointer = func() (int, int) { return c.Input.X, c.Input.Y }
	c.Talk.Sound = func(path string) {
		if c.Env.Sound != nil {
			c.Env.Sound(path)
		}
	}
	c.Chars.Login = c.Login
	c.Password = login.NewInputPassword(c.Env, a)
	c.Password.Notify = c.Login.Notify
	c.Create = login.NewCreateCharacter(c.Env, c.G, c.Net, a, content)
	c.Create.Select, c.Create.Password = c.Chars, c.Password
	c.Chars.Password = c.Password
	c.Create.Notify = c.Login.Notify
	if raw, err := os.ReadFile(a.DataPath(itemExport)); err == nil {
		if items, err := assets.ParseItemCatalogJSON(raw); err == nil {
			lib := role.NewLibraryWith(c.Sprites)
			c.lib, c.items = lib, items
			for i := range c.Chars.Roles {
				c.Chars.Roles[i] = role.NewHuman(lib, items)
			}
			c.Create.Role.Painter = role.NewCreator(lib, items)
		}
	}
	c.Chars.Notify = c.Login.Notify
	c.UI.Add(c.Chars)
	c.UI.Add(c.Create)
	c.UI.Add(c.Password)
	c.Servers.Show()
	return c, nil
}

// Frame is the login phase of DXTimer1Timer.
func (c *Client) Frame() {
	c.handleNet()
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	c.Input.Hovered = nil // FUN_0040f97c
	if c.World != nil {
		c.World.Step(c.Now())
		if c.uiHovered || c.Talk.Contains(c.Input.X, c.Input.Y) {
			c.World.Hover(-1, -1)
		} else {
			c.World.Hover(c.Input.X, c.Input.Y)
		}
		c.reachNPC()
		c.eventTick()
		c.World.HideNames = c.Talk.Drawn()
		if !c.movieFrame() {
			c.World.Draw()
			c.Talk.Draw()
		}
	} else {
		c.drawBackground()
	}
	now := c.Now()
	if !c.Login.LoginTime.IsZero() && now.Sub(c.Login.LoginTime) > loginTimeout {
		c.Notices.Show(noticeTimeout, timeoutNotice, now)
		c.Login.Previous()
	}
	// Web01 (TForm1 +0x4a0) is looked up by name and is missing from this
	// client's pictures, so the original draws nothing at (0, 555).
	// A movie's game mode hides the HUD (and every other form).
	if c.movie == nil {
		c.UI.Tick()
		c.UI.Draw()
	}
	c.Notices.Draw(c.Env, now)
	c.uiHovered = c.Input.Hovered != nil
	c.updateCursor(now)
}

// drawBackground is 0x40c3c8: LogPic1.jpg through the canvas, then the
// logo against the right edge.
func (c *Client) drawBackground() {
	if c.background != nil {
		c.Screen.Draw(0, 0, c.background, false)
	}
	if c.logo == -1 {
		c.logo = c.Pics.Find("Icon_LoginLogo_1")
	}
	if c.logo != -1 {
		w, _ := c.Pics.Size(c.logo)
		c.Pics.Draw(c.Screen, c.logo, ScreenWidth-w, 0, true)
	}
}

// handleNet runs the socket events and the received packets.
func (c *Client) handleNet() {
	for _, e := range c.Net.Poll() {
		switch e.Kind {
		case login.StatusData:
			c.Servers.StatusReceived(e.Data)
		case login.Packet:
			c.dispatch(e.Data)
		case login.Disconnected:
			c.disconnected()
		case login.ConnectFailed:
			c.connectFailed()
		}
	}
}

// dispatch is the part of the packet dispatcher (FUN_002dde1c) the login
// phase reaches.
func (c *Client) dispatch(p []byte) {
	if len(p) == 0 {
		return
	}
	s := p[1:]
	sub := byte(0)
	if len(s) > 0 {
		sub = s[0]
	}
	switch {
	case p[0] == protocol.CommandDiscovery:
		c.disconnectText = login.DisconnectReason(sub, 0)
	case p[0] == protocol.CommandHandshake && sub == protocol.HandshakeServerDescription:
		c.Login.ServerDescription(s)
	case p[0] == protocol.CommandHandshake && sub == handshakeWrongPassword:
		c.wrongPassword()
	case p[0] == protocol.CommandLogin && sub == protocol.LoginRoster:
		c.roster(s)
	case p[0] == protocol.CommandHandshake && sub == protocol.HandshakeWireCode3:
		// 1/3 (0x2ded1a) opens character creation; the byte after the
		// subcommand says the account already has a secret code.
		c.Create.Open(len(s) > 1 && s[1] != 0)
	case p[0] == protocol.CommandMapLoad:
		// AC3 (0x2dfdb3) is the player's own character: the world opens.
		c.enterWorld(s)
	case p[0] == protocol.CommandCharacterState && sub == protocol.CharacterStateWireCode3:
		// 5/3 (FUN_004381c4) fills the player's values.
		c.Stats.ParseBaseStats(s)
	case p[0] == protocol.CommandStats && sub == protocol.StatsStatUpdate && len(s) >= statUpdateBytes:
		// 8/1 (FUN_00416ebc): stat ID, a kind byte, then the value.
		c.Stats.Apply(s[1], binary.LittleEndian.Uint32(s[3:]))
	case p[0] == protocol.CommandGold && sub == protocol.GoldBalance && len(s) >= 5:
		c.Stats.Gold = binary.LittleEndian.Uint32(s[1:])
	case p[0] == protocol.CommandChat && sub == protocol.ChatMapMessage:
		c.mapChat(s)
	case p[0] == protocol.CommandChat && (sub == chatNotice || sub == protocol.ChatHeadBanner):
		// 2/3 and 2/16 (0x2dfd5d): an ID, then text for the notice board.
		if len(s) > chatIDEnd {
			c.Notices.Show(s[chatIDEnd:], chatNoticeFor, c.Now())
		}
	case p[0] == protocol.CommandAppearance:
		c.peerAppears(s)
	case p[0] == protocol.CommandMovement && sub == protocol.MovementMove:
		if id, x, y, ok := world.ParseMove(s); ok && c.World != nil && id != c.World.Player.ID {
			c.World.MovePeer(id, x, y, c.Now())
		}
	case p[0] == protocol.CommandMovement && sub == protocol.MovementMovementLock && len(s) >= 2:
		c.held = s[1] != 0
		if c.held && c.World != nil {
			c.World.StopWalk()
		}
	case p[0] == protocol.CommandEvent && sub >= protocol.EventActorClick && sub <= eventLastFrame:
		c.eventFrame(s)
	case p[0] == protocol.CommandEvent && sub == protocol.EventClose:
		c.eventClose()
	case p[0] == protocol.CommandEvent && sub == protocol.EventResume:
		c.eventResume()
	case p[0] == protocol.CommandPosition:
		if id, _, x, y, ok := world.ParsePlace(s); ok && c.World != nil && id != c.World.Player.ID {
			c.World.PlacePeer(id, x, y)
		}
	case p[0] == protocol.CommandMapAcknowledgment:
		// AC12 (0x2e20cf) places the player on a map.
		c.warp(s)
	case p[0] == protocol.CommandScene && sub == protocol.SceneActorPosition:
		// AC22:4 (FUN_0038cf30) moves, shows and hides map NPCs.
		if c.World != nil {
			c.World.ApplyActorPositions(s[1:])
		}
	case p[0] == protocol.CommandCharacterSelection && sub == protocol.CharacterSelectionDelete:
		// 35/2 (0x2eab55) reports a deletion.
		c.Chars.DeleteResult(s[1:])
	case p[0] == protocol.CommandCharacterCreation && sub == protocol.CharacterCreationCheckName:
		// 9/3 (0x2e1884) answers the name check.
		if len(s) > 1 {
			c.Create.NameResult(s[1])
		}
	default:
		if c.Unhandled != nil {
			c.Unhandled(p)
		}
	}
}

// enterWorld loads the player's map and replaces the login screens with
// the world view.
func (c *Client) enterWorld(p []byte) {
	pl, err := world.ParseSelf(p)
	if err != nil {
		c.Notices.Show([]byte(err.Error()), 3*time.Second, c.Now())
		return
	}
	if c.sceneNames == nil {
		c.sceneNames, _ = world.SceneNames(c.Assets)
		c.mapScenes, _ = world.MapScenes(c.Assets)
	}
	var body login.RoleView
	if c.lib != nil {
		body = role.NewHuman(c.lib, c.items)
	}
	w, err := world.New(c.Env, c.Assets, pl, body, c.sceneNames, func(m uint16) uint16 { return c.mapScenes[m] })
	if err != nil {
		c.Notices.Show([]byte(err.Error()), 3*time.Second, c.Now())
		return
	}
	c.loadNPCs(w)
	w.OnLeg = c.sendLeg
	w.Now = func() time.Time { return c.Now() }
	c.MainStatus.Portrait = w.Body
	c.G.InGame = true
	c.UI.HideAll()
	c.World = w
	c.playMapMusic()
	c.MainStatus.Show()
	c.FuncButtons.Show()
	c.MainButtons.Show()
	c.HotKeys.Show()
	c.ChatBar.Show()
	c.Chat.Show()
	c.welcome()
}

// shadowArchive holds the ground shadow pictures (pic\images.BMg).
const shadowArchive = "images"

// itemExport is the extracted Item.Dat (data/item_data.json).
const itemExport = "item_data.json"

// handshakeWrongPassword is 1/6 (case at 0x2deded). It is not yet named in
// internal/protocol.
const handshakeWrongPassword = 6

// wrongPassword is 1/6: the account form returns with both fields
// cleared. The login timer keeps running, as in the original.
func (c *Client) wrongPassword() {
	if c.G.InGame {
		return
	}
	c.Login.Show()
	c.Login.Account.SetText(nil)
	c.Login.Password.SetText(nil)
	c.Notices.Show(noticeWrongPassword, wrongPasswordNotice, c.Now())
}

// roster is 63/1 (0x2ed7ec): the character selection opens
// (FUN_00402468) and a remembered account is saved.
func (c *Client) roster(s []byte) {
	c.Chars.Roster(s)
	if c.Login.Remember {
		c.Login.Account.AddItem(c.Login.Account.Text)
		c.Login.Account.Save(c.Login.Account.Entries())
	}
}

// disconnected is ClientSocket1Disconnect (0x49871c). The message form
// (PTR_DAT_004ca12c) and the return path behind it (FUN_0049a2e8) are not
// ported: the text is shown as a notice, the login timer stops and the
// server list returns.
func (c *Client) disconnected() {
	c.G.InGame = false
	c.Login.LoginTime = time.Time{}
	text := c.disconnectText
	if len(text) == 0 {
		text = noticeConnection
	}
	c.disconnectText = nil
	c.Login.Hide()
	c.Chars.Hide()
	c.Notices.Show(text, 3*time.Second, c.Now())
	c.Servers.Show()
	if c.Music != nil {
		c.Music.Play(loginMusic) // back in the login phase: CheckStartMusic
	}
}

// connectFailed is ClientSocket1Error (0x49887c).
func (c *Client) connectFailed() {
	c.Net.Close()
	if !c.Servers.Visible {
		c.Notices.Show(noticeConnection, 3*time.Second, c.Now())
		c.Servers.Show()
	}
}
