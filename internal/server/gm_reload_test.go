package server

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/assetsql"
	"wonderland-go/internal/world"
)

func TestGMReloadFailureAndQuestOwnership(t *testing.T) {
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	s.Config.AssetsDatabase = filepath.Join(t.TempDir(), "missing.db")
	before := s.AssetSnapshot()
	say(t, s, p[0], "/reload all")
	if !reflect.DeepEqual(s.AssetSnapshot(), before) || !bytes.Contains(bytes.Join(w[0].packets(t), nil), []byte("reload failed")) {
		t.Fatal("SQL failure changed assets or fell back")
	}
	say(t, s, p[0], "/reload invalid")
	if !reflect.DeepEqual(s.AssetSnapshot(), before) {
		t.Fatal("invalid scope changed assets")
	}
}

func TestGMReloadSQLCategoriesAndConcurrentAdminSnapshots(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB for SQL integration")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("..", "..", path)
	}
	expected, err := assetsql.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	s, p, w := chatFixture(t)
	s.SetGMLevel(p[0].account.ID, 1)
	s.Config.AssetsDatabase = path
	// Keep the test's map geometry but replace just its event definitions.
	for id := range s.Assets.Maps {
		if _, ok := expected.Maps[id]; !ok {
			delete(s.Assets.Maps, id)
		}
	}
	s.World = world.New(s.Assets)
	s.Assets.Mall = []assets.MallItem{{ID: 32176, Name: "Old catalog", Count: 1}}
	s.Assets.Drops = map[uint32][]assets.Drop{}
	before := s.AssetSnapshot()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				snapshot := s.AssetSnapshot()
				_ = snapshot.Summary()
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	say(t, s, p[0], "/reload items")
	if !reflect.DeepEqual(s.AssetSnapshot().Mall, expected.Mall) || !reflect.DeepEqual(before.Mall, []assets.MallItem{{ID: 32176, Name: "Old catalog", Count: 1}}) {
		t.Fatal("mall reload did not publish an immutable SQL snapshot")
	}
	if len(s.Assets.Drops) != 0 {
		t.Fatal("mall reload changed drops")
	}
	say(t, s, p[0], "/reload drop")
	if !reflect.DeepEqual(s.Assets.Drops, expected.Drops) {
		t.Fatal("drop reload did not use SQL")
	}
	p[1].event = &eventSession{}
	for _, wire := range w {
		wire.Reset()
	}
	say(t, s, p[0], "/reload quests")
	if !bytes.Contains(bytes.Join(w[0].packets(t), nil), []byte("finish loading and active interactions")) {
		t.Fatal("quest reload bypassed ownership")
	}
	p[1].event = nil
	old, _ := s.World.Map(10017)
	oldNPCs := len(old.NPCs)
	say(t, s, p[0], "/reload quest")
	updated, _ := s.World.Map(10017)
	if len(updated.NPCs) != oldNPCs || !reflect.DeepEqual(s.Assets.Maps[10017].Events, expected.Maps[10017].Events) {
		t.Fatal("quest reload changed geometry or missed SQL events")
	}
	if err := s.Store.SetGMLevel(context.Background(), p[2].account.ID, 3); err != nil {
		t.Fatal(err)
	}
	say(t, s, p[0], "/reload all")
	if p[2].gmLevel.Load() != 3 || !reflect.DeepEqual(s.Assets.Talks, expected.Talks) || !reflect.DeepEqual(s.Assets.Marks, expected.Marks) {
		t.Fatal("all reload omitted categories")
	}
	for _, packet := range w[0].packets(t) {
		if strings.Contains(string(packet), "reload failed") {
			t.Fatal("unexpected reload error")
		}
	}
}
