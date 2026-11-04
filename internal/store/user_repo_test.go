//go:build integration

package store_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/store"
)

func testDBURL() string {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return "postgres://shortlink:shortlink@localhost:5433/shortlink?sslmode=disable"
	}
	return url
}

func TestUserRepo_CreateAndFind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	repo := store.NewUserRepo(pool)

	uniqueSuffix := uuid.NewString()[:8]
	email := "test_" + uniqueSuffix + "@example.com"

	user := auth.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "$2a$12$fakehashedpasswordforrepointegrationtest",
		CreatedAt:    time.Now().Truncate(time.Microsecond),
	}

	// 1. Create user
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	// 2. Find by ID
	found, err := repo.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID() unexpected error: %v", err)
	}

	if found.ID != user.ID {
		t.Errorf("ID = %v, want %v", found.ID, user.ID)
	}
	if found.Email != user.Email {
		t.Errorf("Email = %q, want %q", found.Email, user.Email)
	}
	if found.PasswordHash != user.PasswordHash {
		t.Errorf("PasswordHash = %q, want %q", found.PasswordHash, user.PasswordHash)
	}

	// 3. Find by Email (case-insensitive check)
	upperEmail := strings.ToUpper(email)
	foundByEmail, err := repo.FindByEmail(ctx, upperEmail)
	if err != nil {
		t.Fatalf("FindByEmail() with upper case email unexpected error: %v", err)
	}
	if foundByEmail.ID != user.ID {
		t.Errorf("found by uppercase email ID = %v, want %v", foundByEmail.ID, user.ID)
	}

	// 4. Duplicate email returns auth.ErrEmailTaken
	dupUser := auth.User{
		ID:           uuid.New(),
		Email:        strings.ToUpper(email), // test case-insensitive uniqueness
		PasswordHash: "$2a$12$someotherhash",
		CreatedAt:    time.Now().Truncate(time.Microsecond),
	}

	dupErr := repo.Create(ctx, dupUser)
	if dupErr == nil {
		t.Fatal("expected duplicate email error, got nil")
	}

	if !errors.Is(dupErr, auth.ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got: %v", dupErr)
	}
}

func TestUserRepo_FindByEmail_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, testDBURL())
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	repo := store.NewUserRepo(pool)

	_, err = repo.FindByEmail(ctx, "nonexistent_"+uuid.NewString()+"@example.com")
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got: %v", err)
	}
}
