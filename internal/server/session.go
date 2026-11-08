package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverlog "wonderland-gonline/internal/server/logging"
	"wonderland-gonline/internal/server/transport"
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
	itemCard              *pendingItemCard
	pendingMail           map[deliveryKey]*transport.Receipt // Guarded by worldMu.
	textMailPending       bool
	adminMailPending      bool
	openTent              *openTent // Guarded by worldMu.
	tentOwner             uint32    // Private scene identity; guarded by worldMu.
	motdSent              bool      // One welcome popup per character login, guarded by worldMu.
	adminFeedback         *[]string // Scoped to a web action under worldMu.
	invisible             bool      // GM ghost mode; guarded by worldMu.
	pendingName           string
	character             *game.Character
	autosaveBaseline      *game.Character       // Last checkpoint; guarded by worldMu.
	battleDisconnect      *battleDisconnectSave // Pending failed final battle checkpoint, guarded by worldMu.
	ready                 bool
	restoreUIAfterMapLoad bool // An event warp cut off its final client acknowledgment; guarded by worldMu.
	// warped marks a portal arrival; peers then receive no AC5:8 login refresh.
	warped               bool
	manufacturing        *store.ManufactureJob
	manufacturingLoaded  bool
	manufactureSync      bool
	tentFloor            byte
	tentSignSync         bool
	tentOccupantsSync    bool
	tentSyncAt           time.Time
	tentFloorClickAt     time.Time
	tentFloorClickX      uint16
	tentFloorClickY      uint16
	fishing              *fishingRun // Transient cast; guarded by worldMu.
	emote                byte        // Current AC32 pose; guarded by worldMu.
	shapeshift           uint16      // Cosmetic NPC disguise, guarded by worldMu.
	restedAt             time.Time   // Last resting recovery while sitting or lying; guarded by worldMu.
	restingLeader        *Session    // Recovery source; changes restart the interval. Guarded by worldMu.
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
	combatOffered        bool // Negotiated transport state; guarded by worldMu.
	combatCapabilities   uint32
	backdropOffered      bool
	clientBackdrops      bool
	connectionOffered    uint32 // Connection-wide feature negotiation; guarded by worldMu.
	connectionFeatures   uint32
	connectionNegotiated bool
	combatSequence       uint32
	encounter            encounterState
	carnieReturn         *world.Destination // Where Carnie's exit portal leads (CarnieReturnMap).
	storageMap           uint16             // NPC storage authority; invalidated by movement or a new interaction.
	hotelMap             uint16
	restMap              uint16 // PendingRestMap: the clinic offered a rest on this map.
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
	resultAck    bool
	info         SessionInfo
	conn         net.Conn
	account      store.Account
	slot         byte
	gmLevel      atomic.Uint32
	sendMu       sync.Mutex
	knownMu      sync.Mutex
	knownPlayers map[uint32]bool // Appearances disclosed to this connection, across map changes.
	log          *slog.Logger
	lastWindow   time.Time
	packets      int
}

func (s *Session) send(p []byte) (err error) {
	if len(p) >= 5 && p[0] == protocol.CommandAppearance {
		defer func() {
			if err == nil {
				s.knownMu.Lock()
				if s.knownPlayers == nil {
					s.knownPlayers = map[uint32]bool{}
				}
				s.knownPlayers[binary.LittleEndian.Uint32(p[1:])] = true
				s.knownMu.Unlock()
			}
		}()
	}
	if output, ok := s.conn.(*transport.Conn); ok {
		return output.Send(p)
	}
	// Socket-free administrator reply capture and deterministic fixture adapters
	// remain synchronous. Accepted gameplay/login sockets always use transport.
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if e := s.conn.SetWriteDeadline(time.Now().Add(transport.WriteTimeout)); e != nil {
		return e
	}
	err = protocol.Write(s.conn, p)
	if s.log != nil {
		s.log.Debug("packet sent", append(serverlog.TraceAttrs(p, true), "session", s.info.ID, "error", err)...)
	}
	return err
}

// Construct before publishing the session: its connection is immutable and all
// existing Close call sites also stop its writer. Launcher status is unframed
// and has no gameplay lock, so it keeps its direct one-response connection.
func (s *Server) networkSession(conn net.Conn, service string, id uint64) *Session {
	c := &Session{conn: conn, log: s.Log, info: SessionInfo{ID: id, Service: service, Remote: conn.RemoteAddr().String(), Connected: time.Now().UTC()}, lastWindow: time.Now()}
	if service != "status" {
		log := s.Log
		c.conn = transport.New(conn, func(frame []byte, err error) {
			if log.Enabled(context.Background(), slog.LevelDebug) {
				p, _ := protocol.Read(bytes.NewReader(frame))
				log.Debug("packet sent", append(serverlog.TraceAttrs(p, true), "session", id, "error", err)...)
			}
			if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
				log.Warn("client write failed", "session", id, "error", err)
			}
		})
	}
	return c
}

func (c *Session) closeAfterFlush() {
	if output, ok := c.conn.(*transport.Conn); ok {
		output.CloseAfterFlush()
	} else {
		c.conn.Close()
	}
}

// Called outside gameplay locks. The writer never acquires server locks or
// performs SQL, and Close interrupts an outstanding network write.
func (c *Session) finishOutput() {
	if output, ok := c.conn.(*transport.Conn); ok {
		if !output.Draining() {
			output.Close()
		}
		<-output.Done()
	} else {
		c.conn.Close()
	}
}

func (c *Session) outputError() error {
	if output, ok := c.conn.(*transport.Conn); ok && !output.Accepting() {
		return net.ErrClosed
	}
	return nil
}

type deliveryKind byte

const (
	deliveryTextMail         deliveryKind = 1
	deliveryAdminMail        deliveryKind = 2
	deliveryWakeCapacity                  = 1
	mailReceiptTimeout                    = 5 * time.Second
	mailReceiptRetryInterval              = time.Second
)

type deliveryKey struct {
	kind     deliveryKind
	receiver uint32
	id       uint64
}

func (s *Server) notifyDeliveryReady() {
	select {
	case s.deliveryReady <- struct{}{}:
	default:
	}
}

// Caller owns worldMu. A receipt on the final frame covers all preceding
// frames in the same ordered connection. Queue acceptance never marks mail.
func (s *Server) sendMailDelivery(ctx context.Context, c *Session, key deliveryKey, packets [][]byte) error {
	if len(packets) == 0 {
		return protocol.ErrMalformed
	}
	var receipt *transport.Receipt
	for i, p := range packets {
		if output, ok := c.conn.(*transport.Conn); ok && i == len(packets)-1 {
			var err error
			receipt, err = output.SendTracked(p, s.notifyDeliveryReady)
			if err != nil {
				return err
			}
		} else if err := c.send(p); err != nil {
			return err
		}
	}
	if receipt == nil {
		return s.markMailDelivered(ctx, key)
	}
	if c.pendingMail == nil {
		c.pendingMail = make(map[deliveryKey]*transport.Receipt)
	}
	c.pendingMail[key] = receipt
	if s.deliveries == nil {
		s.deliveries = make(map[*Session]struct{})
	}
	s.deliveries[c] = struct{}{}
	return nil
}
func (s *Server) markMailDelivered(ctx context.Context, key deliveryKey) error {
	switch key.kind {
	case deliveryTextMail:
		return s.Store.MarkTextMailDelivered(ctx, key.receiver, key.id)
	case deliveryAdminMail:
		return s.Store.MarkAdminMailDelivered(ctx, key.receiver, key.id)
	}
	return errors.New("unknown mail delivery kind")
}

// Completion observations do not touch character state on the writer goroutine.
// Failed SQL markers stay dirty for retry, suppressing duplicate live delivery.
func (s *Server) finishMailReceipts(ctx context.Context, c *Session) {
	for key, receipt := range c.pendingMail {
		if ctx.Err() != nil {
			return
		}
		done, writeErr := receipt.Result()
		if !done {
			continue
		}
		if writeErr == nil {
			if err := s.markMailDelivered(ctx, key); err != nil {
				s.Log.Warn("mail delivery marker deferred", "recipient", key.receiver, "mail", key.id, "error", err)
				continue
			}
		}
		delete(c.pendingMail, key)
	}
}
func (s *Server) processDeliveryReceipts(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, mailReceiptTimeout)
	defer cancel()
	process := func(c *Session) {
		if ctx.Err() != nil {
			return
		}
		if len(c.pendingMail) == 0 && !c.textMailPending && !c.adminMailPending {
			delete(s.deliveries, c)
			return
		}
		s.finishMailReceipts(ctx, c)
		if ctx.Err() != nil {
			return
		}
		if c.character == nil || !c.ready {
			return
		}
		if output, ok := c.conn.(*transport.Conn); ok && !output.Accepting() {
			if len(c.pendingMail) == 0 {
				delete(s.deliveries, c)
			}
			return
		}
		textInFlight, adminInFlight := false, false
		for key := range c.pendingMail {
			textInFlight = textInFlight || key.kind == deliveryTextMail
			adminInFlight = adminInFlight || key.kind == deliveryAdminMail
		}
		if c.textMailPending && !textInFlight {
			if err := s.deliverPendingTextMail(ctx, c); err != nil {
				s.Log.Warn("mail delivery deferred", "recipient", c.character.ID, "error", err)
				if ctx.Err() != nil {
					return
				}
				c.conn.Close()
			}
		}
		if ctx.Err() != nil {
			return
		}
		if c.adminMailPending && !adminInFlight && gmIdle(c) {
			if err := s.deliverAdminMail(ctx, c); err != nil {
				s.Log.Warn("GM mail delivery deferred", "recipient", c.character.ID, "error", err)
				if ctx.Err() != nil {
					c.adminMailPending = true
					return
				}
				c.conn.Close()
			}
		}
	}
	for c := range s.deliveries {
		process(c)
	}
}
func (s *Server) runDeliveryReceipts(ctx context.Context) {
	ticker := time.NewTicker(mailReceiptRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.deliveryReady:
		case <-ticker.C:
		}
		s.processDeliveryReceipts(ctx)
	}
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
	if err := s.saveBattleDisconnect(ctx, c); err != nil {
		return err
	}
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
		// Match exact owned identities, including inactive party members: a
		// pet selection may change while its earlier recovery is still dirty.
		for i := range next.Pets {
			pet := &next.Pets[i]
			for _, saved := range checkpoint.Pets {
				if saved.ID != pet.ID {
					continue
				}
				for _, pending := range old.Pets {
					if pending.ID != pet.ID {
						continue
					}
					if pet.HP == saved.HP && pending.HP > pet.HP {
						pet.HP = min(pending.HP, max(pet.HP, pet.MaxHP))
					}
					if pet.SP == saved.SP && pending.SP > pet.SP {
						pet.SP = min(pending.SP, max(pet.SP, pet.MaxSP))
					}
				}
			}
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
