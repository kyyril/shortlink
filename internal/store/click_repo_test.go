//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/click"
	"github.com/kyyril/shortlink/internal/link"
	"github.com/kyyril/shortlink/internal/store"
)

func newTestLink(t *testing.T, userRepo *store.UserRepo, linkRepo *store.LinkRepo) link.Link {
	t.Helper()
	ctx := context.Background()
	userID := newTestUser(t, userRepo)
	l := link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      "clk_" + uuid.NewString()[:6],
		TargetURL: "https://example.com",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := linkRepo.Insert(ctx, l); err != nil {
		t.Fatalf("newTestLink: %v", err)
	}
	return l
}

func TestClickRepo_BatchInsertAndStats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	userRepo := store.NewUserRepo(pool)
	linkRepo := store.NewLinkRepo(pool)
	clickRepo := store.NewClickRepo(pool)

	testLink := newTestLink(t, userRepo, linkRepo)

	// Base time in UTC: 2 days ago, 1 day ago, today
	now := time.Now().UTC()
	day1 := now.Add(-48 * time.Hour)
	day2 := now.Add(-24 * time.Hour)
	day3 := now

	events := []click.Event{
		{LinkID: testLink.ID, ClickedAt: day1},
		{LinkID: testLink.ID, ClickedAt: day1.Add(time.Hour)},
		{LinkID: testLink.ID, ClickedAt: day2},
		{LinkID: testLink.ID, ClickedAt: day3},
		{LinkID: testLink.ID, ClickedAt: day3.Add(time.Minute)},
		{LinkID: testLink.ID, ClickedAt: day3.Add(2 * time.Minute)},
	}

	// 1. Batch Insert
	if err := clickRepo.InsertClicks(ctx, events); err != nil {
		t.Fatalf("InsertClicks: %v", err)
	}

	// 2. Count Total
	total, err := clickRepo.CountTotal(ctx, testLink.ID)
	if err != nil {
		t.Fatalf("CountTotal: %v", err)
	}
	if total != 6 {
		t.Errorf("total clicks = %d, want 6", total)
	}

	// 3. ClicksPerDay
	// Note: Days with zero clicks are omitted by the SQL query (documented choice).
	since := now.Add(-72 * time.Hour)
	daily, err := clickRepo.ClicksPerDay(ctx, testLink.ID, since)
	if err != nil {
		t.Fatalf("ClicksPerDay: %v", err)
	}

	if len(daily) != 3 {
		t.Fatalf("expected 3 distinct days with clicks, got %d", len(daily))
	}

	if daily[0].Clicks != 2 {
		t.Errorf("day 1 clicks = %d, want 2", daily[0].Clicks)
	}
	if daily[1].Clicks != 1 {
		t.Errorf("day 2 clicks = %d, want 1", daily[1].Clicks)
	}
	if daily[2].Clicks != 3 {
		t.Errorf("day 3 clicks = %d, want 3", daily[2].Clicks)
	}

	// Empty events list is a no-op and succeeds
	if err := clickRepo.InsertClicks(ctx, nil); err != nil {
		t.Errorf("InsertClicks with empty slice returned error: %v", err)
	}
}
