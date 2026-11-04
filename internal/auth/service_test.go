package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/kyyril/shortlink/internal/auth"
)

type fakeRepo struct {
	users map[string]auth.User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: make(map[string]auth.User)}
}

func (f *fakeRepo) Create(ctx context.Context, u auth.User) error {
	normalized := strings.ToLower(u.Email)
	if _, exists := f.users[normalized]; exists {
		return auth.ErrEmailTaken
	}
	f.users[normalized] = u
	return nil
}

func (f *fakeRepo) FindByEmail(ctx context.Context, email string) (auth.User, error) {
	normalized := strings.ToLower(email)
	u, exists := f.users[normalized]
	if !exists {
		return auth.User{}, auth.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (auth.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return auth.User{}, auth.ErrUserNotFound
}

const (
	testJWTSecret = "this-is-a-32-byte-secret-for-testing-1234"
	testJWTTTL    = 15 * time.Minute
)

func TestRegister(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	tests := []struct {
		name     string
		email    string
		password string
		wantErr  error
	}{
		{
			name:     "ValidData_Succeeds",
			email:    "user@example.com",
			password: "password123",
			wantErr:  nil,
		},
		{
			name:     "ShortPassword_ReturnsError",
			email:    "user@example.com",
			password: "short",
			wantErr:  auth.ErrShortPassword,
		},
		{
			name:     "InvalidEmail_NoAt_ReturnsError",
			email:    "invalidemail",
			password: "password123",
			wantErr:  auth.ErrInvalidEmail,
		},
		{
			name:     "InvalidEmail_NoDomain_ReturnsError",
			email:    "user@",
			password: "password123",
			wantErr:  auth.ErrInvalidEmail,
		},
		{
			name:     "InvalidEmail_NoTLD_ReturnsError",
			email:    "user@nodot",
			password: "password123",
			wantErr:  auth.ErrInvalidEmail,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := auth.NewService(repo, testJWTSecret, testJWTTTL, clock)

			u, err := svc.Register(ctx, tc.email, tc.password)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Register() err = %v, want %v", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Register() unexpected error: %v", err)
			}

			if u.Email != tc.email {
				t.Errorf("u.Email = %q, want %q", u.Email, tc.email)
			}

			// Verify bcrypt cost 12
			cost, err := bcrypt.Cost([]byte(u.PasswordHash))
			if err != nil {
				t.Fatalf("bcrypt.Cost error: %v", err)
			}
			if cost != auth.BcryptCost {
				t.Errorf("bcrypt cost = %d, want %d", cost, auth.BcryptCost)
			}
		})
	}
}

func TestRegister_DuplicateEmail_ReturnsErrEmailTaken(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	clock := func() time.Time { return now }

	repo := newFakeRepo()
	svc := auth.NewService(repo, testJWTSecret, testJWTTTL, clock)

	_, err := svc.Register(ctx, "user@example.com", "password123")
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	// Case-insensitive duplicate
	_, err = svc.Register(ctx, "USER@example.com", "password123")
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("second register err = %v, want %v", err, auth.ErrEmailTaken)
	}
}

func TestLogin(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	clock := func() time.Time { return now }

	repo := newFakeRepo()
	svc := auth.NewService(repo, testJWTSecret, testJWTTTL, clock)

	_, err := svc.Register(ctx, "user@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup register failed: %v", err)
	}

	t.Run("ValidCredentials_ReturnsToken", func(t *testing.T) {
		token, err := svc.Login(ctx, "user@example.com", "correct-horse-battery-staple")
		if err != nil {
			t.Fatalf("Login() unexpected error: %v", err)
		}
		if token == "" {
			t.Fatal("expected non-empty token")
		}

		userID, err := svc.VerifyToken(token)
		if err != nil {
			t.Fatalf("VerifyToken() unexpected error: %v", err)
		}
		if userID == uuid.Nil {
			t.Fatal("expected valid user UUID")
		}
	})

	t.Run("WrongPassword_ReturnsErrInvalidCredentials", func(t *testing.T) {
		_, err := svc.Login(ctx, "user@example.com", "wrong-password")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("err = %v, want %v", err, auth.ErrInvalidCredentials)
		}
	})

	t.Run("UnknownEmail_ReturnsSameErrInvalidCredentials", func(t *testing.T) {
		_, err := svc.Login(ctx, "unknown@example.com", "any-password")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("err = %v, want %v", err, auth.ErrInvalidCredentials)
		}
	})
}

func TestVerifyToken(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	currentTime := fixedTime
	clock := func() time.Time { return currentTime }

	repo := newFakeRepo()
	svc := auth.NewService(repo, testJWTSecret, testJWTTTL, clock)

	user, err := svc.Register(ctx, "user@example.com", "password123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	tokenStr, err := svc.IssueToken(user)
	if err != nil {
		t.Fatalf("issue token failed: %v", err)
	}

	t.Run("ValidToken_ReturnsUserID", func(t *testing.T) {
		gotID, err := svc.VerifyToken(tokenStr)
		if err != nil {
			t.Fatalf("VerifyToken() unexpected error: %v", err)
		}
		if gotID != user.ID {
			t.Errorf("gotID = %v, want %v", gotID, user.ID)
		}
	})

	t.Run("ExpiredToken_ReturnsErrTokenExpired", func(t *testing.T) {
		// Advance clock beyond TTL (15m)
		currentTime = fixedTime.Add(16 * time.Minute)

		_, err := svc.VerifyToken(tokenStr)
		if !errors.Is(err, auth.ErrTokenExpired) {
			t.Fatalf("VerifyToken() err = %v, want %v", err, auth.ErrTokenExpired)
		}
		// Reset time
		currentTime = fixedTime
	})

	t.Run("TamperedToken_ReturnsErrTokenInvalid", func(t *testing.T) {
		tampered := tokenStr[:len(tokenStr)-4] + "xxxx"
		_, err := svc.VerifyToken(tampered)
		if !errors.Is(err, auth.ErrTokenInvalid) {
			t.Fatalf("VerifyToken() tampered err = %v, want %v", err, auth.ErrTokenInvalid)
		}
	})

	t.Run("WrongSigningAlgorithm_ReturnsErrTokenInvalid", func(t *testing.T) {
		// Generate an RSA private key to forge an RS256 token
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate rsa key: %v", err)
		}

		claims := jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    auth.TokenIssuer,
			IssuedAt:  jwt.NewNumericDate(fixedTime),
			ExpiresAt: jwt.NewNumericDate(fixedTime.Add(testJWTTTL)),
		}

		rsaToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		signedRSA, err := rsaToken.SignedString(rsaKey)
		if err != nil {
			t.Fatalf("sign rsa token: %v", err)
		}

		_, err = svc.VerifyToken(signedRSA)
		if !errors.Is(err, auth.ErrTokenInvalid) {
			t.Fatalf("VerifyToken() wrong algorithm err = %v, want %v", err, auth.ErrTokenInvalid)
		}
	})
}
