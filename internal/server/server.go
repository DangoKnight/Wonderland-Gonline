// Package server hosts the legacy TCP endpoints and serializes each connection's commands.
package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

var ErrUnsupported = errors.New("action is not ported")

// actorPursuit is transient and guarded by worldMu.
type actorPursuit struct {
	target   uint32
	nextScan time.Time
}

type SessionInfo struct {
	CharacterID   uint32    `json:"character_id,omitempty"`
	CharacterName string    `json:"character_name,omitempty"`
	Map           uint16    `json:"map,omitempty"`
	ID            uint64    `json:"id"`
	Service       string    `json:"service"`
	Remote        string    `json:"remote"`
	Username      string    `json:"username"`
	Connected     time.Time `json:"connected"`
}
type Session struct {
	openTent         *openTent // Guarded by worldMu.
	tentOwner        uint32    // Private scene identity; guarded by worldMu.
	motdSent         bool      // One welcome popup per character login, guarded by worldMu.
	adminFeedback    *[]string // Scoped to a web action under worldMu.
	invisible        bool      // GM ghost mode; guarded by worldMu.
	pendingName      string
	character        *game.Character
	autosaveBaseline *game.Character // Last checkpoint; guarded by worldMu.
	ready            bool
	// warped marks a portal arrival; peers then receive no AC5:8 login refresh.
	warped               bool
	manufacturing        *store.ManufactureJob
	manufacturingLoaded  bool
	fishing              *fishingRun // Transient cast; guarded by worldMu.
	emote                byte        // Current AC32 pose; guarded by worldMu.
	view                 *world.View
	pets                 *petRoster
	event                *eventSession
	arcade               *arcadeSession
	arcadeNextPurchaseAt time.Time
	storm                bool // The ship storm movie is playing (PlayingStormCutscene).
	beachPending         bool
	beach                *beachRun
	battle               *battleRun
	encounter            encounterState
	carnieReturn         *world.Destination // Where Carnie's exit portal leads (CarnieReturnMap).
	restMap              uint16             // PendingRestMap: the clinic offered a rest on this map.
	// Trade state is guarded by worldMu.
	gathering        *gatheringRun     // Guarded by worldMu.
	marriageProposal *marriageProposal // Guarded by worldMu.
	guildInvitation  *guildInvitation  // Guarded by worldMu.
	stall            *playerStall      // Session-owned; guarded by worldMu.
	trade            *tradeSession     // Guarded by worldMu.
	tradeRequest     *tradeRequest
	friendRequests   map[uint32]time.Time // Guarded by worldMu.
	friendOnline     bool                 // Presence survives the map-loading phase of a warp.
	walkMode         byte                 // AC16:4 runtime setting; guarded by worldMu.
	// Team state, guarded by worldMu: the team, pending join requests by
	// requester ID, and the vitals last reported to teammates.
	party         *party
	partyRequests map[uint32]time.Time
	partyVitals   [7]int64
	saleMode      int // NpcSaleMode: -1 closed, 0 equipment, 1 other items.
	saleMap       uint16
	resumeAt      time.Time // NpcClickResumeAt: ignore a repeated click after an interaction.
	lastClick     uint16
	lastClickMap  uint16
	// arrived is set when the character lands on a map (login or warp) and
	// cleared by its first move: the client reports the area it lands in
	// (20/8) before moving, which must not send it back.
	arrived bool
	// resultAck is set by a minigame result: the client follows 57/1 with
	// its own 20/6 (FUN_003bdfa4 marks the event step done), which must not
	// acknowledge the outcome branch's first step.
	resultAck  bool
	info       SessionInfo
	conn       net.Conn
	account    store.Account
	slot       byte
	gmLevel    atomic.Uint32
	sendMu     sync.Mutex
	log        *slog.Logger
	lastWindow time.Time
	packets    int
}

func (s *Session) send(p []byte) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if e := s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); e != nil {
		return e
	}
	err := protocol.Write(s.conn, p)
	if s.log != nil {
		s.log.Debug("packet sent", append(packetTraceAttrs(p, true), "session", s.info.ID, "error", err)...)
	}
	return err
}

type Server struct {
	configPath string // Set before HTTP starts; protected by adminEditMu thereafter.
	bannedIPs  atomic.Value
	ipBanMu    sync.Mutex
	// accountMu orders credential verification/publication against admin changes.
	accountMu    sync.RWMutex
	privilegeMu  sync.Mutex   // Orders persisted and live GM permissions.
	catalogMu    sync.RWMutex // Pre-world and administration snapshots versus reload.
	shutdown     chan struct{}
	shutdownOnce sync.Once
	shutdownAt   time.Time // Guarded by worldMu.
	// worldMu orders visibility, movement, and logout packets across sessions.
	worldMu        sync.Mutex
	world          map[uint64]*Session
	friendSessions map[uint32]*Session // World presence retained during map loading.
	names          map[string]uint64
	Config         config.Config
	Store          *store.Store
	Assets         *assets.Catalog
	World          *world.World
	actorPursuits  map[uint16]map[uint16]*actorPursuit
	Log            *slog.Logger
	Started        time.Time
	mu             sync.Mutex
	sessions       map[uint64]*Session
	accounts       map[uint32]uint64
	listeners      []net.Listener
	wg             sync.WaitGroup
	next           atomic.Uint64
	received       atomic.Uint64
	unsupported    atomic.Uint64
	name           atomic.Value

	logs                      *logBuffer
	comboDamagePerParticipant bool                  // Immutable startup selection, captured by New.
	petGrowthFormula          game.PetGrowthFormula // Immutable startup selection, captured by New.
	expRateMultiplier         float64
	motd                      string
	statusMode                string
	adminEditMu               sync.Mutex
	dropRateMultiplier        float64 // Live GM setting; guarded by worldMu.
}

func New(c config.Config, db *store.Store, a *assets.Catalog, log *slog.Logger) *Server {
	s := &Server{comboDamagePerParticipant: c.ComboDamagePerParticipant, petGrowthFormula: c.PetGrowthFormula, world: map[uint64]*Session{}, friendSessions: map[uint32]*Session{}, names: map[string]uint64{}, Config: c, Store: db, Assets: a, World: world.New(a), Log: log, Started: time.Now(), sessions: map[uint64]*Session{}, accounts: map[uint32]uint64{}}
	s.logs = &logBuffer{}
	if log.Enabled(context.Background(), slog.LevelDebug) {
		s.logs.SetLevel("debug")
	} else {
		s.logs.SetLevel("info")
	}
	s.Log = slog.New(&adminLogHandler{base: log.Handler(), buffer: s.logs})
	s.bannedIPs.Store(map[string]bool{})
	s.shutdown = make(chan struct{})
	s.name.Store(c.Name)
	return s
}
func (s *Server) SetName(name string) { s.name.Store(name) }
func (s *Server) Name() string        { return s.name.Load().(string) }
func (s *Server) Sessions() []SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionInfo, 0, len(s.sessions))
	for _, c := range s.sessions {
		out = append(out, c.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *Server) Kick(id uint64) bool {
	s.mu.Lock()
	c := s.sessions[id]
	s.mu.Unlock()
	if c == nil {
		return false
	}
	c.conn.Close()
	return true
}
func (s *Server) KickAccount(id uint32) {
	s.mu.Lock()
	sid, ok := s.accounts[id]
	s.mu.Unlock()
	if ok {
		s.Kick(sid)
	}
}

// SetGMLevel applies an administrator's GM change to the account's online session.
func (s *Server) SetGMLevel(account uint32, level byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.sessions[s.accounts[account]]; c != nil && c.account.ID == account {
		c.gmLevel.Store(uint32(level))
	}
}
func (s *Server) Counters() (uint64, uint64) { return s.received.Load(), s.unsupported.Load() }

// Run binds every TCP service before accepting any connection; partial startup rolls back.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	services := []struct{ name, address string }{{"login", s.Config.Login}, {"world", s.Config.World}, {"status", s.Config.Status}}
	for _, v := range services {
		l, e := net.Listen("tcp", v.address)
		if e != nil {
			for _, l := range s.listeners {
				l.Close()
			}
			return fmt.Errorf("%s listener: %w", v.name, e)
		}
		s.listeners = append(s.listeners, l)
	}
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.runRespawns(ctx) }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.runLuckyDrawResets(ctx) }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.runAutosave(ctx) }()
	s.wg.Add(1)
	go func() { defer s.wg.Done(); s.runShutdown(ctx) }()
	errs := make(chan error, len(services))
	for i, v := range services {
		s.Log.Info("TCP service listening", "service", v.name, "address", s.listeners[i].Addr())
		s.wg.Add(1)
		go func(name string, l net.Listener) { defer s.wg.Done(); errs <- s.accept(ctx, name, l) }(v.name, s.listeners[i])
	}
	var err error
	select {
	case <-ctx.Done():
	case <-s.shutdown:
	case err = <-errs:
	}
	cancel()
	for _, l := range s.listeners {
		l.Close()
	}
	s.mu.Lock()
	for _, c := range s.sessions {
		c.conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
func (s *Server) accept(ctx context.Context, service string, l net.Listener) error {
	for {
		conn, e := l.Accept()
		if e != nil {
			return e
		}
		if s.ipBanned(conn.RemoteAddr()) {
			conn.Close()
			continue
		}
		s.mu.Lock()
		if ctx.Err() != nil || len(s.sessions) >= s.Config.MaxConnections {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		id := s.next.Add(1)
		c := &Session{conn: conn, log: s.Log, info: SessionInfo{ID: id, Service: service, Remote: conn.RemoteAddr().String(), Connected: time.Now().UTC()}, lastWindow: time.Now()}
		s.sessions[id] = c
		s.wg.Add(1)
		s.mu.Unlock()
		go func() { defer s.wg.Done(); s.serve(ctx, c) }()
	}
}
func (s *Server) serve(ctx context.Context, c *Session) {
	defer func() {
		c.conn.Close()
		s.leaveWorld(c)
		s.releaseName(c)
		s.mu.Lock()
		delete(s.sessions, c.info.ID)
		if s.accounts[c.account.ID] == c.info.ID {
			delete(s.accounts, c.account.ID)
		}
		s.mu.Unlock()
	}()
	if c.info.Service == "status" {
		s.mu.Lock()
		online := len(s.accounts)
		s.mu.Unlock()
		c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		io.Copy(c.conn, bytes.NewReader(s.launcherStatusPacket(online)))
		return
	}
	for {
		if e := c.conn.SetReadDeadline(s.idleReadDeadline(c, time.Now())); e != nil {
			return
		}
		p, e := protocol.Read(c.conn)
		if e != nil {
			if errors.Is(e, io.EOF) {
				s.Log.Info("client closed connection", s.sessionLogAttrs(c)...)
			}
			if !errors.Is(e, io.EOF) && !errors.Is(e, net.ErrClosed) {
				s.Log.Warn("client read failed", append(s.sessionLogAttrs(c), "error", e)...)
			}
			return
		}
		s.Log.Debug("packet received", append(packetTraceAttrs(p, false), "session", c.info.ID)...)
		s.received.Add(1)
		now := time.Now()
		if now.Sub(c.lastWindow) >= time.Second {
			c.lastWindow = now
			c.packets = 0
		}
		c.packets++
		if c.packets > maxPacketsPerSecond {
			s.Log.Warn("packet rate exceeded", "session", c.info.ID)
			return
		}
		e = s.dispatch(ctx, c, p)
		s.partySync(c)
		if errors.Is(e, ErrUnsupported) {
			s.unsupported.Add(1)
			s.Log.Debug("unported action", "session", c.info.ID, "action", p[0])
			continue
		}
		if e != nil {
			s.Log.Warn("client command rejected", append(append(s.sessionLogAttrs(c), packetLogAttrs(p)...), "error", e)...)
			return
		}
	}
}
func (s *Server) dispatch(ctx context.Context, c *Session, p []byte) error {
	if len(p) == 0 {
		return protocol.ErrMalformed
	}
	command := commandRegistry[p[0]]
	if command.policy.IsWorldCommand() {
		return s.worldCommand(ctx, c, p)
	}
	return command.handle(s, ctx, c, p)
}

func (s *Server) login(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.LoginAuthenticate:
		if c.account.ID != 0 {
			return protocol.ErrMalformed
		}
		data := p[2:]
		if len(data) >= 2 {
			version := uint16(data[0]) | uint16(data[1])<<8
			if version >= protocol.LoginClientVersionMin && version <= protocol.LoginClientVersionMax {
				r = protocol.NewReader(data[2:])
			}
		}
		name, password := r.String(), r.String()
		if r.Err() != nil {
			return r.Err()
		}
		// Native BuildLogin appends an XOR key and a length-prefixed item-file
		// size hint. Consume its full layout; it is not an authentication factor.
		if r.Remaining() > 0 {
			size := int(r.U8())
			r.U8() // XOR key; the diagnostic hint is not used by the server.
			r.Bytes(size)
		}
		if r.Err() != nil {
			return r.Err()
		}
		if r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		s.accountMu.RLock()
		defer s.accountMu.RUnlock()
		account, e := s.Store.Authenticate(ctx, name, password)
		if errors.Is(e, store.ErrCredentials) {
			if e = c.send([]byte{protocol.CommandLogin, protocol.LoginSelectAlternate}); e != nil {
				return e
			}
			return c.send([]byte{protocol.CommandHandshake, protocol.HandshakeWireCode6})
		}
		if e != nil {
			return e
		}
		s.mu.Lock()
		if _, ok := s.accounts[account.ID]; ok {
			s.mu.Unlock()
			if e = c.send([]byte{protocol.CommandLogin, protocol.LoginSelectAlternate}); e != nil {
				return e
			}
			return c.send([]byte{protocol.CommandDiscovery, protocol.DiscoveryWireCode19})
		}
		s.accounts[account.ID] = c.info.ID
		c.account = account
		c.gmLevel.Store(uint32(account.GMLevel))
		c.info.Username = account.Username
		s.mu.Unlock()
		if e = c.send(protocol.Builder{protocol.CommandLogin, protocol.LoginSelectAlternate}.U32(account.UserID())); e != nil {
			return e
		}
		chars, e := s.Store.Characters(ctx, account.ID)
		if e != nil {
			return e
		}
		list := protocol.Builder{protocol.CommandLogin, protocol.LoginRoster}
		for _, char := range chars {
			record, e := char.SelectionRecord()
			if e != nil {
				return e
			}
			list = list.Bytes(record)
		}
		if e = c.send(list); e != nil {
			return e
		}
		return c.send([]byte{protocol.CommandCharacterSelection, protocol.CharacterSelectionReady})
	case protocol.LoginCancelCreation:
		// Creation was cancelled: the name reservation and the chosen empty
		// slot are released; the client shows the roster it already has.
		if len(p) != protocol.LoginCancelPacketBytes || c.account.ID == 0 || c.character != nil {
			return protocol.ErrMalformed
		}
		s.releaseName(c)
		c.slot = 0
		return nil
	case protocol.LoginReturn:
		if len(p) != protocol.LoginReturnPacketBytes || c.character != nil {
			return protocol.ErrMalformed
		}
		return s.returnToAccount(c)
	case protocol.LoginSelectAlternate:
		s.catalogMu.RLock()
		defer s.catalogMu.RUnlock()
		if len(p) != protocol.LoginSelectionPacketBytes || c.account.ID == 0 || c.character != nil {
			return protocol.ErrMalformed
		}
		slot := r.U8()
		if r.Err() != nil {
			return r.Err()
		}
		if slot < 1 || slot > 2 {
			return c.send([]byte{protocol.CommandDiscovery, protocol.DiscoveryWireCode32})
		}
		chars, e := s.Store.Characters(ctx, c.account.ID)
		if e != nil {
			return e
		}
		for _, char := range chars {
			if char.Slot == slot {
				char = char.Clone()
				tentRecovered := recoverTentCharacter(&char)
				vitalsChanged := char.RecalculateVitals(s.Assets.Items)
				vehicleChanged := char.NormalizeVehicle(s.Assets.Items)
				unlocks := char.UnlockQualifiedSkills(false, s.hasSkill)
				view, pets := world.NewView(), newPetRoster()
				packets, e := s.worldEntryPackets(char, view, pets)
				if e != nil {
					return e
				}
				if len(unlocks) > 0 || vehicleChanged || vitalsChanged || tentRecovered {
					if e = s.Store.UpdateCharacter(ctx, c.account.ID, char.ID, func(stored *game.Character) error { *stored = char; return nil }); e != nil {
						return e
					}
				}
				s.releaseName(c)
				return s.enterWorld(c, char, view, pets, packets)
			}
		}
		s.releaseName(c)
		c.slot = slot
		hasCode, e := s.Store.HasDeletionCode(ctx, c.account.ID)
		if e != nil {
			return e
		}
		flag := byte(0)
		if hasCode {
			flag = 1
		}
		return c.send([]byte{protocol.CommandHandshake, protocol.HandshakeWireCode3, flag})
	default:
		return ErrUnsupported
	}
}

const (
	statusCrowdedPlayerCount = 10
	statusFullPlayerCount    = 30
)

// StatusPacket is the unframed, unencrypted launcher reply on port 6416.
// Reference: ServerStatusManager.BuildStatusPacket.
// Server IDs must match SERVER.INI: region flag * 100 + server row (1-based).
// Omitted IDs preserve the legacy aliases used by older local configurations.
func StatusPacket(online int, serverIDs ...uint16) []byte {
	if len(serverIDs) == 0 {
		serverIDs = []uint16{protocol.StatusLegacyServerID, protocol.StatusDefaultServerID}
	}
	color := byte(protocol.StatusLoadGreen)
	if online >= statusFullPlayerCount {
		color = protocol.StatusLoadRed
	} else if online >= statusCrowdedPlayerCount {
		color = protocol.StatusLoadYellow
	}
	packet := protocol.Builder{protocol.StatusReplyOpcode, protocol.StatusReplySubtype, protocol.StatusDefaultCluster}
	for _, id := range serverIDs {
		packet = packet.U16(id).U8(color)
	}
	return packet
}
