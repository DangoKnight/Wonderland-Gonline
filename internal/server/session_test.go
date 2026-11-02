package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/server/transport"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func setAutosaveBaseline(c *Session) {
	baseline := c.character.Clone()
	c.autosaveBaseline = &baseline
}

func TestSavedTransactionPreservesPetRecovery(t *testing.T) {
	for _, mode := range []string{"unchanged", "changed_hp", "capped", "removed", "replaced"} {
		t.Run(mode, func(t *testing.T) {
			baseline := game.Character{Pets: []game.Pet{{ID: 123, Slot: 1, HP: 1, SP: 0, MaxHP: 20, MaxSP: 20}}}
			pending, next := baseline.Clone(), baseline.Clone()
			pending.Pets[0].HP, pending.Pets[0].SP = 10, 8
			next.Gold = 99
			switch mode {
			case "changed_hp":
				next.Pets[0].HP = 2
			case "capped":
				next.Pets[0].MaxHP = 5
			case "removed":
				next.Pets = nil
			case "replaced":
				next.Pets[0].ID = 456
			}
			c := &Session{character: &pending, autosaveBaseline: &baseline}
			(&Server{}).adoptSavedCharacter(c, next)
			if c.character.Gold != 99 || c.autosaveBaseline.Gold != 99 {
				t.Fatal("transaction result lost")
			}
			if mode == "removed" {
				if len(c.character.Pets) != 0 {
					t.Fatal("removed pet resurrected")
				}
				return
			}
			wantHP, wantSP := int32(10), int32(8)
			if mode == "changed_hp" {
				wantHP = 2
			} else if mode == "capped" {
				wantHP = 5
			} else if mode == "replaced" {
				wantHP, wantSP = 1, 0
			}
			if c.character.Pets[0].HP != wantHP || c.character.Pets[0].SP != wantSP || c.autosaveBaseline.Pets[0].SP != 0 {
				t.Fatal("pet recovery merge", c.character.Pets, c.autosaveBaseline.Pets)
			}
		})
	}
}

type stalledOutputConn struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (c *stalledOutputConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(p)
}
func queuedPlayer(t *testing.T, s *Server, c *Session) (net.Conn, <-chan struct{}) {
	t.Helper()
	host, client := net.Pipe()
	signal := &stalledOutputConn{Conn: host, started: make(chan struct{})}
	c.conn = s.networkSession(signal, "world", c.info.ID).conn
	t.Cleanup(func() { c.conn.Close(); client.Close(); c.finishOutput() })
	return client, signal.started
}
func awaitSessionProgress(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("session made no progress")
	}
}
func awaitMailWrites(t *testing.T, s *Server, c *Session) {
	t.Helper()
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		s.worldMu.Lock()
		ready := true
		for _, r := range c.pendingMail {
			done, _ := r.Result()
			ready = ready && done
		}
		s.worldMu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("queued mail did not finish writing")
}

func TestQueuedOutputSlowPeerDoesNotHoldWorldLock(t *testing.T) {
	s, players, _ := worldFixture(t)
	s.Assets.Maps[20000] = assets.Map{ID: 20000}
	s.World.ReloadCatalog(s.Assets)
	for _, c := range players {
		c.ready = true
		s.world[c.info.ID] = c
	}
	_, started := queuedPlayer(t, s, players[1])
	result := make(chan error, 1)
	go func() { result <- s.worldCommand(context.Background(), players[0], []byte{6, 1, 2, 0xb0, 4, 0x14, 5}) }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("movement waited for its stalled peer")
	}
	awaitSessionProgress(t, started)
	// The first socket is still blocked. Another map must acquire the same
	// world lock and process its command without waiting for the write deadline.
	go func() { result <- s.worldCommand(context.Background(), players[2], []byte{6, 1, 2, 0xb0, 4, 0x14, 5}) }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow peer blocked an unrelated map")
	}
	if players[2].character.X != 1200 || players[2].character.Y != 1300 {
		t.Fatal("unrelated movement was not applied")
	}
}

func TestQueuedTextMailWaitsForWriteAndRetriesFailedMarker(t *testing.T) {
	s, c, _, path := mallFixture(t)
	client, started := queuedPlayer(t, s, c)
	s.world[c.info.ID] = c
	s.friendSessions[c.character.ID] = c
	ctx := context.Background()
	message, err := s.Store.SendTextMail(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, c.character.ID, 0, []byte("durable receipt"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err = s.deliverPendingTextMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	awaitSessionProgress(t, started)
	s.processDeliveryReceipts(ctx)
	rows, err := s.Store.PendingTextMail(ctx, c.character.ID, 32)
	if err != nil || len(rows) != 1 || rows[0].ID != message.ID {
		t.Fatal("enqueue marked mail delivered", rows, err)
	}
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if p, err := protocol.Read(client); err != nil || !bytes.HasSuffix(p, []byte("durable receipt")) {
		t.Fatal(p, err)
	}
	awaitMailWrites(t, s, c)
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("CREATE TRIGGER reject_delivery BEFORE UPDATE OF delivered ON text_mail BEGIN SELECT RAISE(ABORT,'marker failed'); END"); err != nil {
		t.Fatal(err)
	}
	s.processDeliveryReceipts(ctx)
	rows, err = s.Store.PendingTextMail(ctx, c.character.ID, 32)
	if err != nil || len(rows) != 1 {
		t.Fatal("failed marker reported durable delivery", rows, err)
	}
	// A repeated delivery request must not enqueue the already-written packet
	// again while its SQL marker is dirty.
	s.worldMu.Lock()
	err = s.deliverPendingTextMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	if p, err := protocol.Read(client); err == nil {
		t.Fatal("dirty marker duplicated a live delivery", p)
	} else {
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatal("marker failure disconnected a healthy socket", err)
		}
	}
	if _, err := raw.Exec("DROP TRIGGER reject_delivery"); err != nil {
		t.Fatal(err)
	}
	s.processDeliveryReceipts(ctx)
	rows, err = s.Store.PendingTextMail(ctx, c.character.ID, 32)
	if err != nil || len(rows) != 0 {
		t.Fatal("marker retry failed", rows, err)
	}
}

func TestQueuedMailWorkerContinuesBatchesInOrder(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	client, _ := queuedPlayer(t, s, c)
	s.world[c.info.ID] = c
	s.friendSessions[c.character.ID] = c
	ctx, cancel := context.WithCancel(context.Background())
	for i := 0; i < textMailDeliveryBatchSize+3; i++ {
		if _, err := s.Store.SendTextMail(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, c.character.ID, 0, []byte{byte('A' + i)}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	s.worldMu.Lock()
	err := s.deliverPendingTextMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.runDeliveryReceipts(ctx); close(done) }()
	t.Cleanup(func() { cancel(); awaitSessionProgress(t, done) })
	client.SetReadDeadline(time.Now().Add(3 * time.Second))
	for i := 0; i < textMailDeliveryBatchSize+3; i++ {
		p, err := protocol.Read(client)
		if err != nil || len(p) == 0 || p[len(p)-1] != byte('A'+i) {
			t.Fatal("mail order/batch continuation", i, p, err)
		}
	}
	until := time.Now().Add(2 * time.Second)
	for {
		rows, err := s.Store.PendingTextMail(ctx, c.character.ID, 32)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("worker did not checkpoint successful delivery")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestQueuedGiftWriteFailureRetainsClaimWithoutDuplicateGrant(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	client, started := queuedPlayer(t, s, c)
	s.world[c.info.ID] = c
	s.friendSessions[c.character.ID] = c
	ctx := context.Background()
	before := c.character.Gold
	if err := s.Store.SendAdminMail(ctx, []uint32{c.character.ID}, store.AdminMail{Subject: "Gift", Body: "Queued", Gold: 10, ItemID: 32176, Count: 1}); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err := s.deliverAdminMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	awaitSessionProgress(t, started)
	rows, err := s.Store.PendingAdminMail(ctx, c.character.ID)
	if err != nil || len(rows) != 1 || !rows[0].Claimed || rows[0].Delivered {
		t.Fatal("gift claim/delivery boundary", rows, err)
	}
	if c.character.Gold != before+10 {
		t.Fatal("gift did not commit before queueing")
	}
	s.worldMu.Lock()
	err = s.deliverAdminMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil || c.character.Gold != before+10 {
		t.Fatal("queued gift was granted again", err)
	}
	client.Close()
	c.finishOutput()
	s.processDeliveryReceipts(ctx)
	rows, err = s.Store.PendingAdminMail(ctx, c.character.ID)
	if err != nil || len(rows) != 1 || rows[0].Delivered {
		t.Fatal("failed write cleared durable notification", rows, err)
	}
	// A fresh session's full snapshot restores the committed bag. Redelivering
	// the notification must not run the gift grant a second time.
	reconnected := &Session{conn: &captureConn{}, info: c.info, account: c.account, character: c.character, ready: true, view: world.NewView(), pets: newPetRoster()}
	if err := s.deliverAdminMail(ctx, reconnected); err != nil {
		t.Fatal(err)
	}
	if reconnected.character.Gold != before+10 || adminTestItemCount(reconnected.character.Bag, 32176) != 1 {
		t.Fatal("reconnect duplicated gift")
	}
	rows, err = s.Store.PendingAdminMail(ctx, c.character.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("reconnect notification not checkpointed", rows, err)
	}
}

func TestQueuedDisconnectFlushesNoticeAndStopsWriter(t *testing.T) {
	s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	host, client := net.Pipe()
	c := s.networkSession(host, "world", 1)
	t.Cleanup(func() { c.conn.Close(); client.Close(); c.finishOutput() })
	s.gmKick(c, "Disconnected by test")
	client.SetReadDeadline(time.Now().Add(time.Second))
	if p, err := protocol.Read(client); err != nil || !bytes.Contains(p, []byte("Disconnected by test")) {
		t.Fatal("disconnect notice lost", p, err)
	}
	if _, err := protocol.Read(client); !errors.Is(err, io.EOF) {
		t.Fatal("socket stayed open after notice", err)
	}
	awaitSessionProgress(t, c.conn.(*transport.Conn).Done())
}

func TestQueuedGiftMarkerWaitsForFinalPacket(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	client, _ := queuedPlayer(t, s, c)
	ctx := context.Background()
	if err := s.Store.SendAdminMail(ctx, []uint32{c.character.ID}, store.AdminMail{Subject: "Gift", Body: "Complete", Gold: 10, ItemID: 32176, Count: 1}); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err := s.deliverAdminMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 3; i++ {
		packet, err := protocol.Read(client)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			s.processDeliveryReceipts(ctx)
			rows, err := s.Store.PendingAdminMail(ctx, c.character.ID)
			if err != nil || len(rows) != 1 || !rows[0].Claimed {
				t.Fatal("partial notification marked delivered", rows, err)
			}
		} else if !bytes.HasPrefix(packet, []byte{14, 1}) {
			t.Fatal("notification was not last", packet)
		}
	}
	awaitMailWrites(t, s, c)
	s.processDeliveryReceipts(ctx)
	rows, err := s.Store.PendingAdminMail(ctx, c.character.ID)
	if err != nil || len(rows) != 0 {
		t.Fatal("successful notification not marked", rows, err)
	}
}

func TestQueuedClosingSessionRejectsGameplay(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	queuedPlayer(t, s, c)
	before := c.character.Clone()
	c.closeAfterFlush()
	for _, p := range [][]byte{{6, 1, 2, 0xb0, 4, 0x14, 5}, {12, 1}, {20, 8, 1, 0}} {
		if err := s.worldCommand(context.Background(), c, p); !errors.Is(err, net.ErrClosed) {
			t.Fatal("closing session accepted command", p, err)
		}
	}
	if c.character.X != before.X || c.character.Y != before.Y || c.character.Map != before.Map {
		t.Fatal("closing session changed position")
	}
}

type queuedTestListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func (l *queuedTestListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connections:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *queuedTestListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *queuedTestListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestQueuedAcceptAndShutdownJoinBlockedWriter(t *testing.T) {
	s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	host, client := net.Pipe()
	signal := &stalledOutputConn{Conn: host, started: make(chan struct{})}
	l := &queuedTestListener{connections: make(chan net.Conn, 1), closed: make(chan struct{})}
	l.connections <- signal
	accepted := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { s.accept(ctx, "world", l); close(accepted) }()
	t.Cleanup(func() {
		cancel()
		l.Close()
		client.Close()
		awaitSessionProgress(t, accepted)
		s.mu.Lock()
		for _, c := range s.sessions {
			c.conn.Close()
		}
		s.mu.Unlock()
		joined := make(chan struct{})
		go func() { s.wg.Wait(); close(joined) }()
		awaitSessionProgress(t, joined)
	})
	var c *Session
	until := time.Now().Add(2 * time.Second)
	for c == nil && time.Now().Before(until) {
		s.mu.Lock()
		for _, session := range s.sessions {
			c = session
		}
		s.mu.Unlock()
		if c == nil {
			time.Sleep(time.Millisecond)
		}
	}
	if c == nil {
		t.Fatal("accepted connection not published")
	}
	output, ok := c.conn.(*transport.Conn)
	if !ok {
		t.Fatal("accepted socket bypassed the writer queue")
	}
	if err := c.send([]byte{2, 16, 0}); err != nil {
		t.Fatal(err)
	}
	awaitSessionProgress(t, signal.started)
	// Match Run's shutdown order: stop accepting, close connections, join
	// sessions. No client read is needed to release the in-flight writer.
	cancel()
	l.Close()
	awaitSessionProgress(t, accepted)
	c.conn.Close()
	joined := make(chan struct{})
	go func() { s.wg.Wait(); close(joined) }()
	awaitSessionProgress(t, joined)
	awaitSessionProgress(t, output.Done())
	s.mu.Lock()
	remaining := len(s.sessions)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatal("closed connection remained registered")
	}
}

func TestAutosaveActiveWarpAndFailureIsolation(t *testing.T) {
	s, players, wires := worldFixture(t)
	for _, c := range players {
		setAutosaveBaseline(c)
	}
	s.world[players[0].info.ID] = players[0]
	s.world[players[1].info.ID] = players[1]
	s.friendSessions = map[uint32]*Session{players[0].character.ID: players[0], players[2].character.ID: players[2]}
	// Warping characters remain in the presence map while absent from s.world.
	players[2].warped = true
	players[0].character.X = 1200
	players[1].character.X = 1300
	players[2].character.X = 1400
	// A newer durable reward conflicts with player one's unsaved movement only.
	if err := s.Store.UpdateCharacter(context.Background(), players[0].account.ID, players[0].character.ID, func(c *game.Character) error { c.Gold = 7; return nil }); err != nil {
		t.Fatal(err)
	}
	s.autosaveCharacters(context.Background())
	for i, c := range players {
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if chars[0].Gold != 7 || chars[0].X == 1200 || c.autosaveBaseline.X == 1200 {
				t.Fatal("conflict overwrote state or advanced checkpoint")
			}
		} else if chars[0].X != c.character.X || c.autosaveBaseline.X != c.character.X {
			t.Fatal("other player's save lost", chars)
		}
		if wires[i].Len() != 0 {
			t.Fatal("autosave sent unsolicited packets")
		}
	}
	players[1].character.Y = 1500
	s.leaveWorld(players[1])
	chars, err := s.Store.Characters(context.Background(), players[1].account.ID)
	if err != nil || chars[0].Y != 1500 || players[1].autosaveBaseline != nil {
		t.Fatal("disconnect lost pending state", chars, err)
	}
}

func TestAutosaveWorkerAndSerializedMovement(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	s.world[c.info.ID] = c
	c.ready = true
	c.character.X = 1200
	s.Config.CharacterSaveSeconds = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.runAutosave(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if chars[0].X == 1200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not save pending movement")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker ignored cancellation")
	}
	// World ownership serializes checkpoint reads against gameplay mutation.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.autosaveCharacters(context.Background()) }()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.worldCommand(context.Background(), c, []byte{6, 1, 2, 0xb0, 4, 0x14, 5}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s.autosaveCharacters(context.Background())
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].X != 1200 || chars[0].Y != 1300 {
		t.Fatal(chars, err)
	}
}

func TestBufferedMovementPurchaseAndDisconnect(t *testing.T) {
	s, c, _, path := mallFixture(t)
	setAutosaveBaseline(c)
	s.world[c.info.ID] = c
	old := c.character.Clone()
	// Walking succeeds even while SQL rejects all character writes.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec("CREATE TRIGGER reject_walk BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'unexpected per-packet write'); END"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err = s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 2}.U16(uint16(1100+i)).U16(1200)); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].X != old.X || saved[0].Y != old.Y {
		t.Fatal("walking wrote SQL")
	}
	if err = s.autosaveSession(context.Background(), c); err == nil {
		t.Fatal("failed checkpoint not reported")
	}
	if c.autosaveBaseline.X != old.X {
		t.Fatal("failed checkpoint advanced baseline")
	}
	if _, err = raw.Exec("DROP TRIGGER reject_walk"); err != nil {
		t.Fatal(err)
	}
	x, y := c.character.X, c.character.Y
	tradeDo(t, s, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12}))
	if c.character.X != x || c.character.Y != y {
		t.Fatal("purchase rewound buffered walking")
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 90 {
		t.Fatal(balances, err)
	}
	saved, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Bag != c.character.Bag {
		t.Fatal("purchase not durable", err)
	}
	if saved[0].X != old.X {
		t.Fatal("purchase accidentally flushed walking")
	}
	bag := c.character.Bag
	s.leaveWorld(c)
	saved, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != x || saved[0].Y != y || saved[0].Bag != bag {
		t.Fatal("disconnect lost position or committed inventory", err)
	}
}

func TestBufferedMovementWarpDoesNotRestoreOldPosition(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	c.character.X, c.character.Y = 123, 456
	s.Assets.Maps[20000] = s.Assets.Maps[c.character.Map]
	if err := s.teleport(context.Background(), c, world.Destination{Map: 20000, X: 500, Y: 600}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.autosaveSession(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Map != 20000 || saved[0].X != 500 || saved[0].Y != 600 {
		t.Fatal(saved, err)
	}
}

func TestSavedTransactionPreservesWalkingButAdoptsExplicitPosition(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	baseline := c.character.Clone()
	c.character.X = 123
	reward := baseline.Clone()
	reward.Gold = 7
	s.adoptSavedCharacter(c, reward)
	if c.character.X != 123 || c.character.Gold != 7 || c.autosaveBaseline.X != baseline.X {
		t.Fatal("invalid cache merge")
	}
	warp := reward.Clone()
	warp.X = 789
	s.adoptSavedCharacter(c, warp)
	if c.character.X != 789 || c.autosaveBaseline.X != 789 {
		t.Fatal("committed location ignored")
	}
}

func TestBufferedMovementTradePreservesBothPlayers(t *testing.T) {
	s, players, wires := tradeFixture(t)
	for i, c := range players[:2] {
		tradeDo(t, s, c, protocol.Builder{6, 1, 0}.U16(uint16(1100+i*10)).U16(1200))
	}
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, tradeOffer(0, 1))
	tradeDo(t, s, b, tradeOffer(50))
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
	for i, c := range players[:2] {
		if c.character.X != uint16(1100+i*10) || c.character.Gold != 150 {
			t.Fatal("trade rewound position or failed to transfer", i)
		}
	}
	s.autosaveCharacters(context.Background())
	saved := tradeStored(t, s, players)
	if saved[0].X != 1100 || saved[1].X != 1110 || saved[0].Gold != 150 || saved[1].Gold != 150 || !saved[0].Bag[0].Empty() || saved[1].Bag[1].Count != 10 {
		t.Fatal("checkpoint lost trade or position", saved)
	}
}

func TestCleanCheckpointSkipsSQL(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	// A closed store makes any attempted SQL transaction fail.
	if err := s.Store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.autosaveSession(context.Background(), c); err != nil {
		t.Fatal("clean session queried SQL", err)
	}
}

func TestBufferedMovementHealKeepsPositionDirty(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	c.ready = true
	s.world[c.info.ID] = c
	if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(123).U16(456)); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err := s.heal(context.Background(), c, []string{"heal"})
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s.autosaveCharacters(context.Background())
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != 123 || saved[0].Y != 456 {
		t.Fatal("vitals-only commit hid dirty walking", saved, err)
	}
}

func TestBufferedMovementFullCommitAdoptsCheckpointCoordinates(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	setAutosaveBaseline(c)
	destination := c.character.Clone()
	c.character.X = 123
	if err := s.commitState(context.Background(), c, destination); err != nil {
		t.Fatal(err)
	}
	if c.character.X != destination.X || c.autosaveBaseline.X != destination.X {
		t.Fatal("explicit saved destination was replaced by buffered walking")
	}
}

func TestIdleDeadlineTransitions(t *testing.T) {
	cfg := config.Default()
	s := New(cfg, nil, &assets.Catalog{}, slog.Default())
	c := &Session{}
	now := time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("login deadline = %v", got)
	}
	// A pending creation name remains in the login phase.
	c.pendingName = "New character"
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("creation deadline = %v", got)
	}
	c.character = &game.Character{}
	for _, ready := range []bool{false, true} {
		c.ready = ready
		if got := s.idleReadDeadline(c, now); !got.IsZero() {
			t.Fatalf("gameplay ready=%v retained login deadline: %v", ready, got)
		}
	}
	s.Config.WorldIdleSeconds = 45
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(45 * time.Second)) {
		t.Fatalf("configured gameplay deadline = %v", got)
	}
	c.character = nil
	if got := s.idleReadDeadline(c, now); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("return to login deadline = %v", got)
	}
	s.Config.IdleSeconds = 0
	if got := s.idleReadDeadline(c, now); !got.IsZero() {
		t.Fatalf("disabled login deadline = %v", got)
	}
}

// Accelerate nonzero read deadlines without waiting minutes in socket tests.
// Zero deadlines still reach the real connection unchanged.
type acceleratedIdleConn struct{ net.Conn }

func (c acceleratedIdleConn) SetReadDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		deadline = time.Now().Add(30 * time.Millisecond)
	}
	return c.Conn.SetReadDeadline(deadline)
}

func TestIdleSocketPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		world   bool
		seconds int
		expires bool
	}{
		{"login expires", false, 600, true},
		{"login disabled", false, 0, false},
		{"gameplay disabled", true, 0, false},
		{"gameplay expires", true, 60, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			if tc.world {
				cfg.WorldIdleSeconds = tc.seconds
			} else {
				cfg.IdleSeconds = tc.seconds
			}
			s := New(cfg, nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			client, host := net.Pipe()
			// An inherited login deadline must be cleared when timeout is disabled.
			if err := host.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
			c := s.networkSession(acceleratedIdleConn{host}, "", 1)
			if tc.world {
				c.character = &game.Character{}
			}
			s.sessions[1] = c
			done := make(chan struct{})
			go func() { s.serve(context.Background(), c); close(done) }()
			t.Cleanup(func() {
				client.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("socket cleanup timed out")
				}
			})
			if tc.expires {
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("idle connection did not expire")
				}
			} else {
				select {
				case <-done:
					t.Fatal("disabled idle timeout closed connection")
				case <-time.After(100 * time.Millisecond):
				}
			}
		})
	}
}

func TestDuplicateLoginClosesOnlyRejectedSocket(t *testing.T) {
	s, players, wires := worldFixture(t)
	active := players[0]
	s.accounts[active.account.ID] = active.info.ID
	s.sessions[active.info.ID] = active
	host, client := net.Pipe()
	defer client.Close()
	rejected := s.networkSession(host, "", 3)
	s.sessions[rejected.info.ID] = rejected
	done := make(chan struct{})
	go func() { s.serve(context.Background(), rejected); close(done) }()
	client.SetDeadline(time.Now().Add(10 * time.Second))
	if err := protocol.Write(client, loginPayload(active.account.Username)); err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{{63, 2}, {0, 19}} {
		got, err := protocol.Read(client)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("login rejection %x, %v; want %x", got, err, want)
		}
	}
	if _, err := protocol.Read(client); !errors.Is(err, io.EOF) {
		t.Fatal("rejected socket did not close", err)
	}
	<-done
	if rejected.account.ID != 0 || s.accounts[active.account.ID] != active.info.ID || s.sessions[active.info.ID] != active || wires[0].Len() != 0 {
		t.Fatal("duplicate login changed the active session")
	}
}

func TestConcurrentLoginReservesAccountOnce(t *testing.T) {
	s, players, _ := worldFixture(t)
	account := players[0].account
	var wg sync.WaitGroup
	sessions := []*Session{{conn: &captureConn{}, info: SessionInfo{ID: 3}}, {conn: &captureConn{}, info: SessionInfo{ID: 4}}}
	results := make([]error, len(sessions))
	for i, c := range sessions {
		wg.Add(1)
		go func() { defer wg.Done(); results[i] = s.login(context.Background(), c, loginPayload(account.Username)) }()
	}
	wg.Wait()
	winners, rejections := 0, 0
	for i, c := range sessions {
		switch {
		case results[i] == nil && c.account.ID == account.ID:
			winners++
		case errors.Is(results[i], errAlreadyLoggedIn) && c.account.ID == 0:
			rejections++
		default:
			t.Fatalf("unexpected login result %v, account %d", results[i], c.account.ID)
		}
	}
	if winners != 1 || rejections != 1 {
		t.Fatalf("winners=%d rejections=%d", winners, rejections)
	}
}

func TestLoginReturnReleasesAccountAndCreationName(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	c.character = nil
	ctx := context.Background()
	old := c.account
	s.accounts[old.ID] = c.info.ID
	s.sessions[c.info.ID] = c
	c.info.Username = old.Username
	c.gmLevel.Store(1)
	if _, err := s.reserveName(ctx, c, "Reserved"); err != nil {
		t.Fatal(err)
	}
	c.slot = 2
	tradeDo(t, s, c, []byte{63, 0})
	if wire.Len() != 0 || c.account.ID != 0 || c.slot != 0 || c.pendingName != "" || c.gmLevel.Load() != 0 || c.info.Username != "" {
		t.Fatal("return retained authentication or emitted a reply")
	}
	if _, held := s.accounts[old.ID]; held {
		t.Fatal("old account reservation retained")
	}
	if _, held := s.names["reserved"]; held {
		t.Fatal("creation name retained")
	}
	tradeDo(t, s, c, loginPayload(old.Username))
	if c.account.ID != old.ID || !contains(wire.packets(t), protocol.Builder{63, 2}.U32(old.UserID())) {
		t.Fatal("same-connection reauthentication failed")
	}
	tradeDo(t, s, c, []byte{63, 0})
	tradeDo(t, s, c, loginPayload(players[1].account.Username))
	if c.account.ID != players[1].account.ID {
		t.Fatal("previous account leaked into new login")
	}
}
func TestLoginReturnRejectsWorldAndMalformedRequests(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	old := c.account.ID
	if err := s.dispatch(context.Background(), c, []byte{63, 0}); err == nil {
		t.Fatal("world player returned without world cleanup")
	}
	if c.account.ID != old {
		t.Fatal("rejected request released account")
	}
	c.character = nil
	for _, p := range [][]byte{{63, 0, 1}, {63, 2}, {63, 2, 1, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatal("malformed login transition accepted", p)
		}
	}
	if wire.Len() != 0 || c.account.ID != old {
		t.Fatal("malformed transition mutated state")
	}
}
func TestLoginNativeTailIsFullyDecodedBeforeAuthentication(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	name, accountID := c.account.Username, c.account.ID
	c.account.ID = 0
	c.character = nil
	// Native token is a key plus XOR-encoded ASCII item-file size.
	p, _ := protocol.Builder{63, 4}.U16(1211).String(name)
	p, _ = p.String("password")
	for _, tail := range [][]byte{{0}, {2, 0, 1}, {0, 0, 99}} {
		if err := s.dispatch(context.Background(), c, append(append([]byte{}, p...), tail...)); err == nil {
			t.Fatal("malformed native tail accepted", tail)
		}
		if c.account.ID != 0 || wire.Len() != 0 {
			t.Fatal("malformed tail authenticated or emitted packets")
		}
	}
	tradeDo(t, s, c, append(p, 3, 0x55, '1'^0x55, '2'^0x55, '3'^0x55))
	if c.account.ID != accountID || wire.Len() == 0 {
		t.Fatal("native login tail rejected")
	}
}
func TestLoginRosterUsesNativeSelectionLayout(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	name := c.account.Username
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(char *game.Character) error {
		char.HP = 12
		char.MaxHP = 100
		char.SP = 8
		char.MaxSP = 50
		for i := range char.Equipment {
			char.Equipment[i] = game.Item{ID: uint16(0x1001 + i), Count: 1}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c.account.ID = 0
	c.character = nil
	tradeDo(t, s, c, loginPayload(name))
	packets := wire.packets(t)
	if len(packets) != 3 || !bytes.Equal(packets[1][:2], []byte{63, 1}) {
		t.Fatal(packets)
	}
	record := packets[1][2:]
	n := int(record[1])
	if len(record) != 54+n || !bytes.Equal(record[n+40:n+44], []byte{0, 0, 1, 0x10}) {
		t.Fatal("rebirth/job/equipment misaligned", record)
	}
}

// TestCancelCreationReleasesNameAndSlot covers 63/3, sent when the client's
// creation wizard is cancelled on its first step (0x2d8dcd).
func TestCancelCreationReleasesNameAndSlot(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	c.character = nil
	if _, err := s.reserveName(context.Background(), c, "Reserved"); err != nil {
		t.Fatal(err)
	}
	c.slot = 2
	old := c.account.ID
	tradeDo(t, s, c, []byte{63, 3})
	if wire.Len() != 0 || c.slot != 0 || c.pendingName != "" || c.account.ID != old {
		t.Fatal("cancel kept the slot or name, logged out, or replied")
	}
	if _, held := s.names["reserved"]; held {
		t.Fatal("creation name retained")
	}
	for _, p := range [][]byte{{63, 3, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatal("malformed cancel accepted", p)
		}
	}
}

func TestTerminateSessionReleasesAccountForReconnect(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(ctx, "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	s := New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	connect := func(id uint64) (net.Conn, <-chan struct{}) {
		client, host := net.Pipe()
		c := &Session{conn: host, info: SessionInfo{ID: id}, lastWindow: time.Now()}
		s.mu.Lock()
		s.sessions[id] = c
		s.mu.Unlock()
		done := make(chan struct{})
		go func() { s.serve(ctx, c); close(done) }()
		t.Cleanup(func() {
			client.Close()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("session cleanup timed out")
			}
		})
		client.SetDeadline(time.Now().Add(5 * time.Second))
		p, _ := protocol.Builder{63, 4}.String("tester")
		p, _ = p.String("password")
		if err := protocol.Write(client, p); err != nil {
			t.Fatal(err)
		}
		for _, prefix := range []byte{63, 63, 35} {
			p, err := protocol.Read(client)
			if err != nil || p[0] != prefix {
				t.Fatal("login", p, err)
			}
		}
		return client, done
	}
	client, done := connect(1)
	if !s.Kick(1) {
		t.Fatal("connected session not terminated")
	}
	if _, err := protocol.Read(client); err == nil {
		t.Fatal("terminated connection still readable")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("termination did not clean up")
	}
	if s.Kick(1) || len(s.Sessions()) != 0 {
		t.Fatal("terminated session remained published")
	}
	if got, err := db.Authenticate(ctx, "tester", "password"); err != nil || got.ID != account.ID {
		t.Fatal("termination changed account", err)
	}
	_, done = connect(2)
	if !s.Kick(2) {
		t.Fatal("reconnected session missing")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnected session cleanup timed out")
	}
}
