package server

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverlog "wonderland-gonline/internal/server/logging"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

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
	restedAt             time.Time   // Last resting recovery while sitting or lying; guarded by worldMu.
	view                 *world.View
	pets                 *petRoster
	event                *eventSession
	arcade               *arcadeSession
	arcadeNextPurchaseAt time.Time
	stormDefinition      *assets.StoryCinematic
	stormAck             bool
	stormStarted         time.Time
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
	bazaarQuotes     map[uint32]store.Bazaar
	stall            *playerStall         // Session-owned; guarded by worldMu.
	trade            *tradeSession        // Guarded by worldMu.
	friendRequests   map[uint32]time.Time // Guarded by worldMu.
	friendOnline     bool                 // Presence survives the map-loading phase of a warp.
	walkMode         byte                 // AC16:4 runtime setting; guarded by worldMu.
	// Team state, guarded by worldMu: the team, pending join requests by
	// requester ID, and the vitals last reported to teammates.
	party         *party
	partyRequests map[uint32]time.Time
	partyVitals   [8]int64
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
		s.log.Debug("packet sent", append(serverlog.TraceAttrs(p, true), "session", s.info.ID, "error", err)...)
	}
	return err
}

func (s *Server) sendAll(c *Session, packets [][]byte) error {
	for _, p := range packets {
		if e := c.send(p); e != nil {
			return e
		}
	}
	return nil
}

// commit persists a changed copy of the character, then adopts it in the session.
func (s *Server) commit(ctx context.Context, c *Session, next game.Character) error {
	old := *c.character
	if err := s.commitState(ctx, c, next); err != nil {
		return err
	}
	if old.ActiveVehicle != 0 && (old.ActiveVehicle != c.character.ActiveVehicle || old.VehicleSlot != c.character.VehicleSlot) {
		return s.sendVehiclePackets(c, [][]byte{serverwire.VehicleDismountPacket(&old)})
	}
	return nil
}

// commitState saves and adopts state without publishing packets. The raft wreck
// path supplies its own break/dismount order. Callers hold worldMu.
func (s *Server) commitState(ctx context.Context, c *Session, next game.Character) error {
	if err := game.PreserveItemLocks(*c.character, &next); err != nil {
		return err
	}
	next.NormalizeVehicle(s.Assets.Items)
	if e := s.Store.UpdateCharacter(ctx, c.account.ID, next.ID, func(stored *game.Character) error {
		*stored = next
		return nil
	}); e != nil {
		return e
	}
	changedOfferState := c.character.Bag != next.Bag || c.character.Gold != next.Gold
	// This transaction saved the complete supplied snapshot, including its
	// position. Unlike a database result that retains the old position, an
	// explicit full-state commit must adopt its destination exactly.
	*c.character = next
	baseline := next.Clone()
	c.autosaveBaseline = &baseline
	if changedOfferState {
		s.closeStall(c)
		s.cancelTrade(c)
	}
	return nil
}

const (
	autosaveTimeout = 5 * time.Second
)

// Caller holds worldMu, which also serializes gameplay commits and disconnects.
func (s *Server) autosaveSession(ctx context.Context, c *Session) error {
	if c.character == nil || c.autosaveBaseline == nil {
		return nil
	}
	next := c.character.Clone()
	if store.CharacterSnapshotsEqual(*c.autosaveBaseline, next) {
		return nil
	}
	_, err := s.Store.AutosaveCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: next.ID}, *c.autosaveBaseline, next)
	if err != nil {
		return err
	}
	c.autosaveBaseline = &next
	return nil
}

func (s *Server) autosaveCharacters(ctx context.Context) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, autosaveTimeout)
	defer cancel()
	// Presence retains characters during warp loading; include those sessions.
	seen := make(map[*Session]bool)
	save := func(c *Session) {
		if c == nil || seen[c] || ctx.Err() != nil {
			return
		}
		seen[c] = true
		if err := s.autosaveSession(ctx, c); err != nil {
			s.Log.Error("character autosave failed", "session", c.info.ID, "character", c.character.ID, "error", err)
		}
	}
	for _, c := range s.world {
		save(c)
	}
	for _, c := range s.friendSessions {
		save(c)
	}
}

func (s *Server) runAutosave(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(s.Config.CharacterSaveSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.autosaveCharacters(ctx)
		}
	}
}

// adoptSavedCharacter publishes a committed SQL result, retaining unsaved walking
// only when the transaction left the durable position unchanged. Explicit moves
// adopt their committed destination. Caller holds worldMu.
func (s *Server) adoptSavedCharacter(c *Session, next game.Character) {
	baseline := next.Clone()
	if c.character != nil && c.autosaveBaseline != nil {
		old, checkpoint := c.character, c.autosaveBaseline
		if old.Map == checkpoint.Map && next.Map == checkpoint.Map && next.X == checkpoint.X && next.Y == checkpoint.Y {
			next.X, next.Y = old.X, old.Y
		}
		// Resting recovery is buffered like walking; keep it unless the
		// transaction set the vital itself.
		if next.HP == checkpoint.HP && old.HP > next.HP {
			next.HP = min(old.HP, max(next.HP, next.MaxHP))
		}
		if next.SP == checkpoint.SP && old.SP > next.SP {
			next.SP = min(old.SP, max(next.SP, next.MaxSP))
		}
	}
	*c.character = next
	c.autosaveBaseline = &baseline
}

// idleReadDeadline uses the login limit until a character is selected or
// created. Map loading and warps retain the gameplay limit even while not ready.
// A zero timeout clears the previous deadline so idle gameplay stays connected.
func (s *Server) idleReadDeadline(c *Session, now time.Time) time.Time {
	timeout := s.Config.IdleTimeout()
	s.worldMu.Lock()
	if c.character != nil {
		timeout = s.Config.WorldIdleTimeout()
	}
	s.worldMu.Unlock()
	if timeout == 0 {
		return time.Time{}
	}
	return now.Add(timeout)
}

// returnToAccount handles bare AC63:0 from character selection. The client
// opens its account form locally and expects no reply or connection closure.
func (s *Server) returnToAccount(c *Session) error {
	// Match authentication's lock order so admin credential changes cannot race
	// release and subsequent authentication on this connection.
	s.accountMu.RLock()
	defer s.accountMu.RUnlock()
	s.releaseName(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accounts[c.account.ID] == c.info.ID {
		delete(s.accounts, c.account.ID)
	}
	c.account = store.Account{}
	c.gmLevel.Store(0)
	c.slot = 0
	c.info.Username = ""
	c.info.CharacterID, c.info.CharacterName, c.info.Map = 0, "", 0
	return nil
}
