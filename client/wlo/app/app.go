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
	"wonderland-gonline/internal/game"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/settings"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/team"
	"wonderland-gonline/client/wlo/weather"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
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
	options           Options
	Root              string
	Assets            login.Assets
	resources         *Resources
	backgroundSession bool
	profileAccount    string

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
	World              *world.World
	Settings           *settings.Form
	SettingsState      *settings.State
	settingsPromptForm *seui.Form
	settingsPath       string
	Inventory          *inventory.Form
	Compound           *inventory.CompoundForm
	Team               *team.Form
	TeamState          *team.State
	teamAppearances    map[uint32]*world.Peer
	joinTeamTarget     bool
	remote             remoteRuntime
	InventoryState     *inventory.State
	Stats              *world.Stats     // the player's values (5/3, 8/1, 26/4)
	MainStatus         *hud.MainStatus  // the status panel over the world
	FuncButtons        *hud.FuncBtnForm // the toolbar at the top right
	MainButtons        *hud.MainBtnForm // the menu at the bottom right
	HotKeys            *hud.HotKeyForm  // the F1–F8 bar
	ChatBar            *hud.InputBar    // the chat bar
	groundHeld         bool             // the left button went down on the ground
	groundSince        time.Time        // when it went down
	uiHovered          bool             // a control was under the pointer last frame
	nextWalk           time.Time        // next re-plan while a button or arrow is held
	Chat               *hud.ChatLog     // the chat log over the map
	npcTemplates       map[uint32]world.NPCTemplate
	Talk               *hud.Talk // the event talk window
	Music              *Music    // background music, nil without audio
	movie              *moviePlay
	sport              *sportPlay // the running minigame
	sceneMusic         map[uint16]string
	talks              map[uint16]string
	event              eventState
	petAnnouncements   [][]byte
	petAnnouncement    bool
	pendingNPC         *world.NPC // clicked out of reach, sent on arrival
	areas              areaWatch
	sounds             []string // the sound table (soundtable.go)
	sfx                *Sounds  // the effects player (set by Run)
	ambient            ambience
	// mapReady is +0x133d0: cleared by the player's AC12, set by 5/4 after
	// the map load is acknowledged. Until then prop sounds and area
	// triggers stay silent, so the arrival's replay of opened props does
	// not sound.
	mapReady bool
	held     bool // 6/2: the server holds the player

	lib            *role.Library
	items          map[uint16]assets.NativeItem
	Skills         *skills.Form
	SkillState     *skills.State
	hotbar         hotbarRuntime
	waterTravel    waterTravelState
	vehicleEffects vehicleEffectState
	logoutPrompt   *logoutForm
	// Scene names (SceneData.dat) and each map's scene (eve.Emg), read on
	// first entry.
	sceneNames map[uint16]string
	mapScenes  map[uint16]uint16
	// The weather layer (PTR_DAT_004ca2b0), shared by the maps and the
	// movies, and each scene's weather byte.
	weather      *weather.Layer
	quietWhisper bool              // +0x1b4: a portrait press, so no "Whisp to" line
	players      map[uint32][]byte // known online players by ID (chat.go)
	sceneWeather map[uint16]byte

	background *surface.Surface // login background object (0x40c27c)
	logo       int              // its Icon_LoginLogo_1
	// disconnectText is DAT_00828128 +0x18, left by an action-0 packet.
	disconnectText []byte
	// Now is the frame clock; tests replace it.
	Now func() time.Time
	// Exit is set by the Leave button.
	Exit bool
	// Lost is the message form a lost connection shows; fade darkens and
	// freezes the scene behind it (lost.go).
	Lost *LostForm
	fade fadeState
	// Unhandled receives packets the login phase does not handle.
	Unhandled func(p []byte)
}

// Options configure New.
type Options struct {
	Renderer string // auto (default), gpu rasterization, or cpu compatibility rendering
	// SpritesRoot is an optional directory of sprite packs or of the
	// editable export, searched before the decompiled data root's sprites/.
	Shared       *Resources // workspace-owned immutable assets and decoded caches
	UserRoot     string     // writable preferences for this session
	SpritesRoot  string
	Root         string // the asset directory (login.Assets)
	SettingsPath string // optional local preference file override
	ServerINI    string // SERVER.INI override
	FallbackINI  []byte // used when SERVER.INI cannot be read
}

// New is the login part of FormCreate: the picture database with the
// default and white skins, the font, the form manager in the white skin's
// colour, the login background and the two login forms.
func New(o Options) (*Client, error) {
	r := o.Shared
	if r == nil {
		r = &Resources{}
	}
	c := &Client{Root: o.Root, Assets: login.NewAssets(o.Root), Now: time.Now, resources: r, options: o}
	c.Assets.UserRoot = o.UserRoot
	a := c.Assets
	if err := r.prepare(a, o.SpritesRoot); err != nil {
		return nil, err
	}
	os.MkdirAll(filepath.Dir(a.UserPath("save.dat")), 0o755)
	c.Screen = surface.New(ScreenWidth, ScreenHeight)
	c.Pics = r.pics
	c.Cursors = &cursor.Cursors{Shapes: r.cursors}
	c.Cursors.Set(cursor.ShapeNormal, c.Now())
	c.Input = &seui.Input{}
	c.Env = &seui.Env{Pics: c.Pics, Screen: c.Screen, Text: r.textRenderer}
	c.UI = seui.NewManager(c.Env, c.Input)
	c.UI.SetColor(skinColor)
	c.background = r.background
	c.logo = -1

	c.G = login.NewGlobals()
	c.Net = login.NewNet()
	c.Servers = login.NewSelectServer(c.Env, c.G, c.Net, a)
	c.Servers.INI, c.Servers.FallbackINI = o.ServerINI, o.FallbackINI
	c.Servers.OnExit = func() { c.Exit = true }
	c.Lost = newLostForm(c.Env)
	c.UI.Add(c.Lost)
	c.Lost.Leave.OnClick = func() { c.Exit = true }
	c.Lost.Prev.OnClick = c.lostPrev
	c.Servers.OnConnecting = func() { c.clearProfileAccount(); c.Net.Send(login.Discovery()) }
	c.UI.Add(c.Servers)
	c.Login = login.NewIDPassword(c.Env, c.G, c.Net, a)
	c.Login.Account.Clipboard = loginClipboardText
	c.Login.Password.Clipboard = loginClipboardText
	c.Login.OnLogout = c.clearProfileAccount
	c.Login.Servers = c.Servers
	c.Login.Notify = func(text []byte, d time.Duration) { c.Notices.Show(text, d, c.Now()) }
	c.UI.Add(c.Login)
	formula := r.formula
	c.Sprites = r.sprites
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
	c.ChatBar.Message.Now = func() time.Time { return c.Now() }
	c.ChatBar.Whisper.OnEnter = c.whisperEnter
	c.ChatBar.Whisper.OnBlur = c.whisperBlur
	c.ChatBar.OnWhisperName = c.whisperEnter
	c.ChatBar.Pointer = func() (int, int) { return c.Input.X, c.Input.Y }
	c.ChatBar.Focus = func(f seui.Control) { c.Input.Focused = f }
	c.ChatBar.InTeam, c.ChatBar.InGuild = c.inTeam, c.inGuild
	c.Chat = hud.NewChatLog(c.Env)
	c.Chat.Now = func() time.Time { return c.Now() }
	c.Chat.Faces = c.chatFace
	c.Chat.SelfID = func() uint32 {
		if c.World == nil {
			return 0
		}
		return c.World.Player.ID
	}
	c.Chat.OnSpeaker = func(id uint32) {
		c.quietWhisper = true
		c.whisperTo(id)
		c.quietWhisper = false
	}
	c.Chat.Pointer = c.ChatBar.Pointer
	c.ChatBar.Notice = c.Chat.Notice
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
	c.lib, c.items = r.library, r.items
	if c.items != nil {
		for i := range c.Chars.Roles {
			c.Chars.Roles[i] = role.NewHuman(c.lib, c.items)
		}
		c.Create.Role.Painter = role.NewCreator(c.lib, c.items)
	}
	c.initInventory()
	c.initCompound()
	c.initSkills()
	c.initTeam()
	c.initSettings(o.SettingsPath)
	c.Chars.Notify = c.Login.Notify
	c.UI.Add(c.Chars)
	c.UI.Add(c.Create)
	c.UI.Add(c.Password)
	c.Servers.Show()
	return c, nil
}

// Frame is the login phase of DXTimer1Timer.
func (c *Client) Frame() { c.frame(true) }

// frame always pumps networking, simulation and automation; background
// thumbnails render at a lower rate. Movies/minigames retain their own clocks.
func (c *Client) frame(draw bool) {
	draw = draw || c.movie != nil || c.sport != nil || c.fade.step > 0
	c.handleNet()
	if draw {
		c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	}
	c.Input.Hovered = nil // FUN_0040f97c
	if c.World != nil && c.fade.frozen == nil {
		c.World.JoinTeamSelection = c.joinTeamTarget
		c.World.Step(c.Now())
		c.waterTravelTick()
		c.remoteTick()
		if c.uiHovered || c.Talk.Contains(c.Input.X, c.Input.Y) {
			c.World.Hover(-1, -1)
		} else {
			c.World.Hover(c.Input.X, c.Input.Y)
		}
		c.reachNPC()
		c.eventTick()
		c.areaTick()
		c.ambientTick()
		// The lost connection's snapshot is taken before the names and the
		// location line are drawn, so the frozen scene has neither.
		fading := c.fade.step > 0
		c.World.HideNames = c.Talk.Drawn() || fading
		c.World.Cinematic = fading
		if !c.movieFrame() && draw {
			c.World.Draw()
			c.drawVehicleEffects()
			c.Talk.Draw()
		}
		c.sportFrame()
	} else if c.fade.frozen == nil && draw {
		c.drawBackground()
	}
	now := c.Now()
	c.drawFade(now)
	if !c.Login.LoginTime.IsZero() && now.Sub(c.Login.LoginTime) > loginTimeout {
		c.Notices.Show(noticeTimeout, timeoutNotice, now)
		c.Login.Previous()
	}
	// Web01 (TForm1 +0x4a0) is looked up by name and is missing from this
	// client's pictures, so the original draws nothing at (0, 555).
	// A movie's game mode hides the HUD (and every other form).
	if c.movie == nil {
		c.UI.Tick()
		if draw {
			settingsVisible := c.Settings.Visible
			if c.logoutPrompt != nil {
				c.Settings.Visible = false
			}
			c.UI.Draw()
			c.Settings.Visible = settingsVisible
			if c.logoutPrompt != nil && c.logoutPrompt.Visible {
				c.Screen.FillAlpha(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0, logoutShadeAlpha)
				c.logoutPrompt.Update(c.Input)
			}
			if c.Inventory != nil {
				c.Inventory.DrawDragged()
				c.Compound.DrawDragged()
				c.drawHotbarDrag()
			}
		}
	}
	if draw {
		c.remoteInformation()
		c.Notices.Draw(c.Env, now)
	}
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
	if c.teamPacket(p) {
		return
	}
	if c.hotbarPacket(p) {
		return
	}
	if c.skillPacket(p) {
		return
	}
	if c.remoteBattlePacket(p) {
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
		c.rememberProfileAccount()
		c.Create.Open(len(s) > 1 && s[1] != 0)
	case p[0] == protocol.CommandMapLoad:
		// AC3 (0x2dfdb3) is the player's own character: the world opens.
		c.enterWorld(s)
	case p[0] == protocol.CommandCharacterState && sub == protocol.CharacterStateWireCode3:
		// 5/3 (FUN_004381c4) fills the player's values.
		if !c.SkillState.Snapshot(p) {
			if c.Unhandled != nil {
				c.Unhandled(p)
			}
			return
		}
		c.Stats.ParseBaseStats(s)
		c.Skills.Refresh()
	case p[0] == protocol.CommandSettings:
		c.settingsPacket(p)
	case p[0] == protocol.CommandPetControl && (sub == protocol.PetControlVehicleMount || sub == protocol.PetControlWireCode11 || sub == protocol.PetControlRemoveVehicle || sub == protocol.PetControlVehiclePosition):
		c.vehiclePacket(p)
	case p[0] == protocol.CommandPetControl && sub == protocol.PetControlWireCode8:
		if c.World != nil {
			before := c.InventoryState.Pets
			if c.InventoryState.ApplyPetList(p) {
				c.announceNewPets(before)
				c.assignPetSkills()
				c.Skills.Refresh()
			} else if c.Unhandled != nil {
				c.Unhandled(p)
			}
		}
	case p[0] == protocol.CommandPetControl && sub == protocol.PetControlPetSlot:
		if c.World != nil && len(p) == 7 && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID && p[6] >= 1 && p[6] <= game.MaxPets {
			c.InventoryState.Pets[p[6]-1] = inventory.UsePet{}
			c.Skills.Refresh()
		}
	case p[0] == protocol.CommandInventory:
		c.inventoryPacket(p)
	case p[0] == protocol.CommandStats && sub == protocol.StatsWireCode2:
		if c.World != nil {
			if c.InventoryState.ApplyPetStat(p) {
				c.Skills.Refresh()
			} else if c.Unhandled != nil {
				c.Unhandled(p)
			}
		}
	case p[0] == protocol.CommandStats && sub == protocol.StatsStatUpdate && len(s) >= statUpdateBytes && (len(s) < statTargetEnd || binary.LittleEndian.Uint32(s[statTargetOffset:]) == 0):
		// 8/1 (FUN_00416ebc): stat ID, a kind byte, then the value.
		c.Stats.Apply(s[1], binary.LittleEndian.Uint32(s[3:]))
		if s[1] == world.StatPoints && c.Inventory != nil {
			c.Inventory.AllocationReply()
		}
	case p[0] == protocol.CommandGold && sub == protocol.GoldBalance && len(s) >= 5:
		c.Stats.Gold = binary.LittleEndian.Uint32(s[1:])
	case p[0] == protocol.CommandChat && sub <= protocol.ChatAllyMessage:
		c.receiveChat(sub, s)
	case p[0] == protocol.CommandChat && sub == protocol.ChatHeadBanner:
		// 2/16 (0x2dfd44): an ID, then text for the notice board.
		if len(s) > chatIDEnd {
			c.Notices.Show(s[chatIDEnd:], chatNoticeFor, c.Now())
		}
	case p[0] == protocol.CommandPresence:
		c.receivePresence(s)
	case p[0] == protocol.CommandPose:
		c.receivePose(s)
	case p[0] == protocol.CommandCharacterState && sub == protocol.CharacterStateEquipmentSnapshot:
		if c.World != nil {
			c.World.ApplyPeerEquipment(s[1:])
		}
	case p[0] == protocol.CommandCharacterState && sub == protocol.CharacterStateSpriteRefresh:
		if len(s) == 6 && c.World != nil {
			if peer := c.World.Peers[binary.LittleEndian.Uint32(s[1:])]; peer != nil {
				world.Dress(peer.Role, peer.Player)
			}
		}
	case p[0] == protocol.CommandAppearance:
		c.peerAppears(s)
	case p[0] == protocol.CommandMovement && sub == protocol.MovementMove:
		if id, _, _, ok := world.ParseMove(s); ok && c.World != nil && id != c.World.Player.ID {
			c.World.ApplyPeerMovement(s, c.Now())
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
	case p[0] == protocol.CommandEvent && stepDone(sub):
		// 20/10 and its twins finish the step (+0x7108): the frame loop
		// acknowledges it with 20/6, which moves server-driven sequences
		// such as the beach rescue along.
		c.event.done = true
	case p[0] == protocol.CommandMinigame && sub == protocol.MinigameStart:
		c.startSport(s)
	case p[0] == protocol.CommandArcadeGame:
		if c.sport != nil {
			if g, ok := c.sport.game.(interface{ Receive([]byte, time.Time) bool }); ok {
				g.Receive(s, c.Now())
			}
		}
	case p[0] == protocol.CommandMinigame && sub == protocol.MinigameEnd:
		c.endSport()
	case p[0] == protocol.CommandPosition:
		if id, mapID, x, y, ok := world.ParsePlace(s); ok && c.World != nil {
			if id == c.World.Player.ID && mapID == c.World.Player.Map {
				c.rememberVehiclePosition(id)
				c.waterTravel = waterTravelState{}
				c.World.Relocate(image.Pt(x, y))
				c.World.MarkWalk()
			} else if id != c.World.Player.ID {
				c.rememberVehiclePosition(id)
				c.World.PlacePeer(id, x, y)
			}
		}
	case p[0] == protocol.CommandMapAcknowledgment:
		// AC12 (0x2e20cf) places the player on a map.
		c.warp(s)
	case p[0] == protocol.CommandCharacterState && sub == protocol.CharacterStateRefresh:
		// 5/4 (FUN_002dde1c's 5/4 case) marks the map ready.
		c.mapReady = true
	case p[0] == protocol.CommandScene && sub == protocol.SceneActorState:
		// AC22:1 (FUN_00408054) sets an NPC's frame; an opening prop sounds
		// once the map is ready.
		if c.World != nil {
			if snd := c.World.ApplyActorState(s[1:]); snd != 0 && c.mapReady && c.Env.Sound != nil {
				c.Env.Sound(numberedSound(int(snd)))
			}
		}
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
	c.vehicleEffects = vehicleEffectState{}
	c.waterTravel = waterTravelState{}
	pl, err := world.ParseSelf(p)
	if err != nil {
		c.Notices.Show([]byte(err.Error()), 3*time.Second, c.Now())
		return
	}
	if c.sceneNames == nil {
		c.resources.loadScenes(c.Assets)
		c.sceneNames, c.mapScenes = c.resources.sceneNames, c.resources.mapScenes
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
	c.attachWeather(w)
	w.OnLeg = c.sendLeg
	w.Now = func() time.Time { return c.Now() }
	c.MainStatus.Portrait = w.Body
	c.G.InGame = true
	c.UI.HideAll()
	c.World = w
	c.resetInventory(pl)
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
	c.clearProfileAccount()
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
	c.rememberProfileAccount()
	c.Chars.Roster(s)
	if c.Login.Remember {
		c.Login.Account.AddItem(c.Login.Account.Text)
		c.Login.Account.Save(c.Login.Account.Entries())
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

const statTargetOffset, statTargetEnd = 7, 11 // AC8:1 target after stat, kind and value; zero denotes self.
