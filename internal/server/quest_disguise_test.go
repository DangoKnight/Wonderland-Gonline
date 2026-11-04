package server

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestQuestDisguiseWire(t *testing.T) {
	c := &Session{character: &game.Character{ID: 0x01020304}, shapeshift: 14531}
	if got := shapeshiftPacket(c); !bytes.Equal(got, []byte{5, 5, 4, 3, 2, 1, 195, 56}) {
		t.Fatal("shape wire changed", got)
	}
	c.shapeshift = 0
	if got := shapeshiftPacket(c); !bytes.Equal(got, []byte{5, 5, 4, 3, 2, 1, 0, 0}) {
		t.Fatal("shape reset changed", got)
	}
}

func TestInstalledPrincessDisguises(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	before := c.character.Clone()
	count := 0
	for _, mid := range []uint16{12250, 12251, 12257} {
		m := catalog.Maps[mid]
		for _, ev := range m.Events {
			for bi, b := range ev.Branches {
				for _, raw := range b.Operations {
					op := world.DecodeOp(raw)
					if op.Code != world.ActionTransform {
						continue
					}
					count++
					es := &eventSession{mapID: mid, ev: &ev, branch: bi}
					c.event = es
					if !s.transformationAllowed(mid, &ev, bi, op) {
						t.Fatal("native disguise still rejected", mid, ev.ClickID, b.Index)
					}
					ok, err := s.transformStoryCharacter(context.Background(), c, es, op)
					if !ok || err != nil || c.shapeshift != op.D2 || !reflect.DeepEqual(before, *c.character) {
						t.Fatal("temporary shape changed durable character", err)
					}
				}
			}
		}
	}
	if count != 13 {
		t.Fatal("missing native disguise coverage", count)
	}
}

func TestInstalledWikiActorCapabilities(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	count := 0
	for _, rule := range catalog.Story.Rules {
		if rule.Kind != "actor_capability" || len(rule.Scope.Events) != 1 || len(rule.Scope.Branches) != 1 || len(rule.Scope.Maps) != 1 || !strings.HasPrefix(rule.ID, "wiki_actor_") {
			continue
		}
		count++
		mid, eid, bid := rule.Scope.Maps[0], rule.Scope.Events[0], rule.Scope.Branches[0]
		ev, known := s.World.Event(mid, eid)
		if !known {
			t.Fatal("missing actor event", rule.ID)
		}
		found := false
		for bi, b := range ev.Branches {
			if b.Index == bid {
				for _, raw := range b.Operations {
					if !rule.Pattern.Matches(raw.Data) {
						continue
					}
					found = true
					es := &eventSession{mapID: mid, ev: ev, branch: bi}
					if !s.actorCapability(c, es, world.DecodeOp(raw)) {
						t.Fatal("native actor action not enabled", rule.ID)
					}
				}
			}
		}
		if !found {
			t.Fatal("actor fingerprint lost native operation", rule.ID)
		}
	}
	if count != 877 {
		t.Fatal("incomplete actor profile", count)
	}
}
