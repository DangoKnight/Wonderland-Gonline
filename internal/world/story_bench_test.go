package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

var storyBenchmarkRule assets.StoryRule
var storyBenchmarkMatch bool
var storyBenchmarkEvent *assets.Event
var storyBenchmarkCount int

func BenchmarkStory(b *testing.B) {
	c := &game.Character{Quests: map[uint32]game.Quest{13046: {ID: 13046, State: game.InProgress, Step: 1}}}
	ev := assets.Event{ClickID: 6, Branches: []assets.Branch{{Index: 6, Operations: []assets.Operation{action(1, 3, 2, 14081, 0, 0), action(2, 2, 1, 0, 10001, 0)}}}}
	w := New(&assets.Catalog{Story: assets.DefaultStoryDefinitions(), Maps: map[uint16]assets.Map{12052: {ID: 12052, Events: []assets.Event{ev}}}})
	native, _ := w.Event(12052, 6)
	for _, tc := range []struct {
		name, kind string
		context    StoryContext
	}{
		{"Movie", assets.StoryMovieMode, StoryContext{Map: 12465, Event: 2}},
		{"Visibility", assets.StoryVisibility, StoryContext{Map: 12000, Actor: 28}},
		{"Miss", assets.StoryMovieMode, StoryContext{Map: 65000, Event: 1}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				storyBenchmarkRule, storyBenchmarkMatch = w.StoryFirst(tc.kind, c, nil, tc.context)
			}
		})
	}
	b.Run("AllVisibility", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			n := 0
			for r := range w.StoryRules(assets.StoryVisibility, c, nil, StoryContext{Map: 12000, Actor: 28}) {
				if r.Value != 0 {
					n++
				}
			}
			storyBenchmarkCount = n
		}
	})
	b.Run("StaticRepair", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			storyBenchmarkEvent = w.PrepareStory(c, nil, 12052, native, 0)
		}
	})
}
