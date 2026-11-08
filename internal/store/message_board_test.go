package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMessageBoardExpiresAfterOneDay(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "board.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.orm.Exec("INSERT INTO accounts(id,username,password_hash,email,created_at) VALUES(1,'board','x','','')").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.orm.Exec("INSERT INTO characters(id,account_id,slot,name,state) VALUES(10001,1,1,'Author',X'00')").Error; err != nil {
		t.Fatal(err)
	}
	ref := CharacterRef{Account: 1, ID: 10001}
	now := time.Now()
	var expired MessageBoardPost
	for i := 0; i < MessageBoardPostsPerAuthor; i++ {
		expired, err = s.CreateMessageBoardPost(ctx, ref, []byte(fmt.Sprint(i)), []byte("Old"), now.Add(-MessageBoardLifetime-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.MessageBoardEntry(ctx, expired.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("expired message still readable", err)
	}
	if posts, count, err := s.MessageBoardPage(ctx, 0, 10); err != nil || count != 0 || len(posts) != 0 {
		t.Fatal("expired messages still listed", posts, count, err)
	}
	// An identical expired notice must neither consume quota nor deduplicate a
	// new post. It receives a fresh ID without renewing the old timestamp.
	fresh, err := s.CreateMessageBoardPost(ctx, ref, expired.Title, expired.Content, now)
	if err != nil || fresh.ID <= expired.ID {
		t.Fatal("expiry did not release author quota", fresh, err)
	}
	if posts, count, err := s.MessageBoardPage(ctx, 0, 10); err != nil || count != 1 || len(posts) != 1 || posts[0].ID != fresh.ID {
		t.Fatal(posts, count, err)
	}
	// Exactly 24 hours is expired; one millisecond before that is still live.
	for _, sample := range []struct {
		age  time.Duration
		want int64
	}{{MessageBoardLifetime - time.Millisecond, 1}, {MessageBoardLifetime, 0}} {
		var count int64
		err := activeMessageBoardPosts(s.orm, now.Add(sample.age)).Model(&MessageBoardPost{}).Where("id = ?", fresh.ID).Count(&count).Error
		if err != nil || count != sample.want {
			t.Fatal(sample.age, count, err)
		}
	}
}

func TestMessageBoardMutationsAuthenticateAndEnforceQuota(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	// Identity-only fixture: board mutations must not rewrite character stats.
	if err := s.orm.Exec("INSERT INTO accounts(id,username,password_hash,email,created_at) VALUES(1,'board','x','','')").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.orm.Exec("INSERT INTO characters(id,account_id,slot,name,state) VALUES(10001,1,1,'Author',X'00'),(4510001,1,2,'Other',X'00')").Error; err != nil {
		t.Fatal(err)
	}
	ref := CharacterRef{Account: 1, ID: 10001}
	first, err := s.CreateMessageBoardPost(ctx, ref, []byte("One"), []byte("Body"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateMessageBoardPost(ctx, ref, []byte("One"), []byte("Body"), time.Now())
	if err != nil || again.ID != first.ID {
		t.Fatal("retry duplicated notice", again, err)
	}
	for _, title := range []string{"Two", "Three"} {
		if _, err := s.CreateMessageBoardPost(ctx, ref, []byte(title), []byte("Body"), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateMessageBoardPost(ctx, ref, []byte("Four"), []byte("Body"), time.Now()); !errors.Is(err, ErrMessageBoardAuthorLimit) {
		t.Fatal(err)
	}
	if err := s.DeleteMessageBoardPost(ctx, CharacterRef{Account: 1, ID: 4510001}, first.ID); err == nil {
		t.Fatal("another character deleted a post")
	}
	if err := s.DeleteMessageBoardPost(ctx, CharacterRef{Account: 2, ID: 10001}, first.ID); err == nil {
		t.Fatal("spoofed account deleted a post")
	}
	if err := s.DeleteMessageBoardPost(ctx, ref, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessageBoardPost(ctx, ref, []byte("Four"), []byte("Body"), time.Now()); err != nil {
		t.Fatal("deleting did not release quota", err)
	}
	if _, err := s.CreateMessageBoardPost(ctx, ref, []byte(" "), []byte("Body"), time.Now()); !errors.Is(err, ErrMessageBoardText) {
		t.Fatal(err)
	}
}

func TestMessageBoardConcurrentQuota(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.orm.Exec("INSERT INTO accounts(id,username,password_hash,email,created_at) VALUES(1,'board','x','','')").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.orm.Exec("INSERT INTO characters(id,account_id,slot,name,state) VALUES(10001,1,1,'Author',X'00')").Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := s.CreateMessageBoardPost(context.Background(), CharacterRef{Account: 1, ID: 10001}, []byte(fmt.Sprint(index)), []byte("Body"), time.Now())
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrMessageBoardAuthorLimit) {
			t.Fatal(err)
		}
	}
	if success != 3 {
		t.Fatal("concurrent posts bypassed quota", success)
	}
}

func TestMessageBoardPersistsAndPages(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "board.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint32{1, 2, 3} {
		post := MessageBoardPost{ID: id, AuthorName: []byte("Author"), Title: []byte("Title"), Content: []byte("Body"), PostedAtMillis: time.Now().UnixMilli() + int64(id)}
		if err := s.orm.Create(&post).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Re-run a preserving upgrade on an existing database with real content.
	if _, err := s.db.Exec("PRAGMA user_version=21"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	posts, count, err := s.MessageBoardPage(ctx, 1, 1)
	if err != nil || count != 3 || len(posts) != 1 || posts[0].ID != 2 {
		t.Fatal(posts, count, err)
	}
	post, err := s.MessageBoardEntry(ctx, 1)
	if err != nil || string(post.Content) != "Body" {
		t.Fatal(post, err)
	}
	if _, _, err := s.MessageBoardPage(ctx, -1, 10); err == nil {
		t.Fatal("negative offset accepted")
	}
	if _, _, err := s.MessageBoardPage(ctx, 0, 11); err == nil {
		t.Fatal("unbounded page accepted")
	}
}
