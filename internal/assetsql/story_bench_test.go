package assetsql

import (
	"gorm.io/gorm"
	"reflect"
	"sync/atomic"
	"testing"
	"wonderland-gonline/internal/assets"
)

var storySQLBenchmarkResult assets.StoryDefinitions

func BenchmarkStorySQLLoad(b *testing.B) {
	defs := assets.DefaultStoryDefinitions()
	db := storyReadFixture(b, defs)
	var queries atomic.Int64
	if err := db.Callback().Query().After("gorm:query").Register("benchmark:story_queries", func(*gorm.DB) { queries.Add(1) }); err != nil {
		b.Fatal(err)
	}
	read := func() error {
		return db.Transaction(func(tx *gorm.DB) error { var err error; storySQLBenchmarkResult, err = readStory(tx); return err })
	}
	if err := read(); err != nil || !reflect.DeepEqual(storySQLBenchmarkResult, defs) {
		b.Fatal("story projection", err)
	}
	queries.Store(0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := read(); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(queries.Load())/float64(b.N), "queries/op")
}
