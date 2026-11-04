//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/link"
	"github.com/kyyril/shortlink/internal/store"
)

// newTestUser is a helper that creates a user in the DB and returns its ID.
func newTestUser(t *testing.T, repo *store.UserRepo) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	u := auth.User{
		ID:           uuid.New(),
		Email:        "link_test_" + uuid.NewString()[:8] + "@example.com",
		PasswordHash: "$2a$12$fakehashedpasswordforrepointegrationtest",
		CreatedAt:    time.Now().Truncate(time.Microsecond),
	}

	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("newTestUser: create user: %v", err)
	}

	return u.ID
}

func TestLinkRepo_InsertAndGet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	userRepo := store.NewUserRepo(pool)
	linkRepo := store.NewLinkRepo(pool)
	userID := newTestUser(t, userRepo)

	code := "test_" + uuid.NewString()[:6]
	target := "https://example.com/" + uuid.NewString()

	l := link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      code,
		TargetURL: target,
		ExpiresAt: nil,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	// 1. Insert
	if err := linkRepo.Insert(ctx, l); err != nil {
		t.Fatalf("Insert() unexpected error: %v", err)
	}

	// 2. GetByCode
	got, err := linkRepo.GetByCode(ctx, code)
	if err != nil {
		t.Fatalf("GetByCode() unexpected error: %v", err)
	}
	if got.Code != code {
		t.Errorf("Code = %q, want %q", got.Code, code)
	}
	if got.TargetURL != target {
		t.Errorf("TargetURL = %q, want %q", got.TargetURL, target)
	}
	if got.UserID != userID {
		t.Errorf("UserID = %v, want %v", got.UserID, userID)
	}

	// 3. GetOwned by correct user
	owned, err := linkRepo.GetOwned(ctx, code, userID)
	if err != nil {
		t.Fatalf("GetOwned() unexpected error: %v", err)
	}
	if owned.Code != code {
		t.Errorf("GetOwned Code = %q, want %q", owned.Code, code)
	}

	// 4. GetOwned by other user returns ErrNotFound
	otherUserID := newTestUser(t, userRepo)
	_, err = linkRepo.GetOwned(ctx, code, otherUserID)
	if !errors.Is(err, link.ErrNotFound) {
		t.Errorf("GetOwned by other user: expected ErrNotFound, got: %v", err)
	}
}

func TestLinkRepo_DuplicateCode_ReturnsErrCodeTaken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	userRepo := store.NewUserRepo(pool)
	linkRepo := store.NewLinkRepo(pool)
	userID := newTestUser(t, userRepo)

	code := "dup_" + uuid.NewString()[:6]

	l := link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      code,
		TargetURL: "https://example.com",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := linkRepo.Insert(ctx, l); err != nil {
		t.Fatalf("Insert first: %v", err)
	}

	// Same code, different ID
	l2 := link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      code,
		TargetURL: "https://other.com",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	err = linkRepo.Insert(ctx, l2)
	if !errors.Is(err, link.ErrCodeTaken) {
		t.Errorf("expected ErrCodeTaken, got: %v", err)
	}
}

func TestLinkRepo_DeleteOwned(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	userRepo := store.NewUserRepo(pool)
	linkRepo := store.NewLinkRepo(pool)
	userID := newTestUser(t, userRepo)

	code := "del_" + uuid.NewString()[:6]
	l := link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      code,
		TargetURL: "https://example.com",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	if err := linkRepo.Insert(ctx, l); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := linkRepo.DeleteOwned(ctx, code, userID); err != nil {
		t.Fatalf("DeleteOwned: %v", err)
	}

	// Should be gone
	_, err = linkRepo.GetByCode(ctx, code)
	if !errors.Is(err, link.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestLinkRepo_ListByUser_KeysetPagination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	userRepo := store.NewUserRepo(pool)
	linkRepo := store.NewLinkRepo(pool)
	userID := newTestUser(t, userRepo)

	// Insert 3 links with slightly different timestamps so ordering is deterministic.
	var inserted []link.Link
	for i := range 3 {
		l := link.Link{
			ID:        uuid.New(),
			UserID:    userID,
			Code:      "page_" + uuid.NewString()[:6],
			TargetURL: "https://example.com",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Millisecond).Truncate(time.Microsecond),
		}
		if err := linkRepo.Insert(ctx, l); err != nil {
			t.Fatalf("Insert link %d: %v", i, err)
		}
		inserted = append(inserted, l)
	}

	// First page: limit 2 — should return 2 newest links.
	page1, err := linkRepo.ListByUser(ctx, userID, nil, 2)
	if err != nil {
		t.Fatalf("ListByUser page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	// Build cursor from last item on page 1.
	cursor := &link.Cursor{
		CreatedAt: page1[len(page1)-1].CreatedAt,
		ID:        page1[len(page1)-1].ID,
	}

	// Second page — should return the remaining 1 link.
	page2, err := linkRepo.ListByUser(ctx, userID, cursor, 2)
	if err != nil {
		t.Fatalf("ListByUser page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}

	// Verify no overlap: IDs from page2 should not appear in page1.
	seen := map[uuid.UUID]bool{}
	for _, l := range page1 {
		seen[l.ID] = true
	}
	for _, l := range page2 {
		if seen[l.ID] {
			t.Errorf("link %v appeared on both pages", l.ID)
		}
	}

	// Total unique links seen = 3
	if len(page1)+len(page2) != 3 {
		t.Errorf("total links seen = %d, want 3", len(page1)+len(page2))
	}
}
