package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestGMLevelsIncludesAccountsBeyondAdminList(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var rows []accountRow
	for i := 0; i < adminAccountListLimit+1; i++ {
		rows = append(rows, accountRow{Account: Account{Username: fmt.Sprintf("gm-%d", i), GMLevel: byte(i % 3)}})
	}
	if err := s.orm.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	levels, err := s.GMLevels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != adminAccountListLimit+1 {
		t.Fatal("privilege reload was truncated", len(levels))
	}
	var last accountRow
	if err := s.orm.Order("id DESC").First(&last).Error; err != nil {
		t.Fatal(err)
	}
	if levels[last.ID] != last.GMLevel {
		t.Fatal("last account omitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.GMLevels(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
}
