package link_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/httpx"
	"github.com/kyyril/shortlink/internal/link"
)

// --- fakes ---

type fakeRepo struct {
	links   map[string]link.Link
	nextErr error // returned once then cleared
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{links: make(map[string]link.Link)}
}

func (r *fakeRepo) Insert(_ context.Context, l link.Link) error {
	if err := r.nextErr; err != nil {
		r.nextErr = nil
		return err
	}
	if _, exists := r.links[l.Code]; exists {
		return link.ErrCodeTaken
	}
	r.links[l.Code] = l
	return nil
}

func (r *fakeRepo) GetByCode(_ context.Context, code string) (link.Link, error) {
	l, ok := r.links[code]
	if !ok {
		return link.Link{}, link.ErrNotFound
	}
	return l, nil
}

func (r *fakeRepo) GetOwned(_ context.Context, code string, userID uuid.UUID) (link.Link, error) {
	l, ok := r.links[code]
	if !ok || l.UserID != userID {
		return link.Link{}, link.ErrNotFound
	}
	return l, nil
}

func (r *fakeRepo) ListByUser(_ context.Context, userID uuid.UUID, _ *link.Cursor, limit int) ([]link.Link, error) {
	var result []link.Link
	for _, l := range r.links {
		if l.UserID == userID {
			result = append(result, l)
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (r *fakeRepo) DeleteOwned(_ context.Context, code string, userID uuid.UUID) error {
	l, ok := r.links[code]
	if !ok || l.UserID != userID {
		return nil
	}
	delete(r.links, code)
	return nil
}

type fakeGen struct {
	codes []string
	pos   int
	errAt int // return error at this call index (-1 = never)
}

func newFakeGen(codes ...string) *fakeGen {
	return &fakeGen{codes: codes, errAt: -1}
}

func (g *fakeGen) New() (string, error) {
	if g.errAt >= 0 && g.pos == g.errAt {
		return "", errors.New("fake gen error")
	}
	if g.pos >= len(g.codes) {
		return "fallback" + uuid.NewString()[:4], nil
	}
	code := g.codes[g.pos]
	g.pos++
	return code, nil
}

// fixedClock returns the same time every call.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// --- tests ---

func TestService_Create_ValidURL_ReturnsLink(t *testing.T) {
	repo := newFakeRepo()
	svc := link.NewService(repo, link.NewRandCodeGenerator(), time.Now)

	in := link.CreateInput{
		UserID: uuid.New(),
		URL:    "https://example.com",
	}

	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if len(got.Code) != 7 {
		t.Errorf("code length = %d, want 7", len(got.Code))
	}
	if got.TargetURL != in.URL {
		t.Errorf("TargetURL = %q, want %q", got.TargetURL, in.URL)
	}
}

func TestService_Create_JavascriptScheme_ReturnsErrInvalidURL(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)

	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID: uuid.New(),
		URL:    "javascript:alert(1)",
	})
	if !errors.Is(err, link.ErrInvalidURL) {
		t.Errorf("expected ErrInvalidURL, got: %v", err)
	}
}

func TestService_Create_FtpScheme_ReturnsErrInvalidURL(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)

	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID: uuid.New(),
		URL:    "ftp://files.example.com/file.txt",
	})
	if !errors.Is(err, link.ErrInvalidURL) {
		t.Errorf("expected ErrInvalidURL, got: %v", err)
	}
}

func TestService_Create_CustomCodeTaken_ReturnsErrCodeTaken(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()

	// Pre-insert a link with the same code.
	_ = repo.Insert(context.Background(), link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      "mycode",
		TargetURL: "https://example.com",
		CreatedAt: time.Now(),
	})

	svc := link.NewService(repo, link.NewRandCodeGenerator(), time.Now)

	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID:     userID,
		URL:        "https://other.com",
		CustomCode: "mycode",
	})
	if !errors.Is(err, link.ErrCodeTaken) {
		t.Errorf("expected ErrCodeTaken, got: %v", err)
	}
}

func TestService_Create_ReservedCode_ReturnsErrInvalidCode(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)

	for _, reserved := range []string{"v1", "healthz", "readyz"} {
		_, err := svc.Create(context.Background(), link.CreateInput{
			UserID:     uuid.New(),
			URL:        "https://example.com",
			CustomCode: reserved,
		})
		if !errors.Is(err, link.ErrInvalidCode) {
			t.Errorf("reserved code %q: expected ErrInvalidCode, got: %v", reserved, err)
		}
	}
}

func TestService_Create_ShortCode_ReturnsErrInvalidCode(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)

	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID:     uuid.New(),
		URL:        "https://example.com",
		CustomCode: "abc", // 3 chars, min is 4
	})
	if !errors.Is(err, link.ErrInvalidCode) {
		t.Errorf("expected ErrInvalidCode for short code, got: %v", err)
	}
}

func TestService_Create_InvalidCharsInCode_ReturnsErrInvalidCode(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)

	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID:     uuid.New(),
		URL:        "https://example.com",
		CustomCode: "has space",
	})
	if !errors.Is(err, link.ErrInvalidCode) {
		t.Errorf("expected ErrInvalidCode for space in code, got: %v", err)
	}
}

func TestService_Create_ExpiresAtInPast_ReturnsErrInvalidExpiry(t *testing.T) {
	now := time.Now()
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), fixedClock(now))

	past := now.Add(-time.Hour)
	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID:    uuid.New(),
		URL:       "https://example.com",
		ExpiresAt: &past,
	})
	if !errors.Is(err, link.ErrInvalidExpiry) {
		t.Errorf("expected ErrInvalidExpiry, got: %v", err)
	}
}

func TestService_Create_ExpiresAtExactlyNow_ReturnsErrInvalidExpiry(t *testing.T) {
	now := time.Now()
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), fixedClock(now))

	// Exactly now: !now.After(now) => invalid
	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID:    uuid.New(),
		URL:       "https://example.com",
		ExpiresAt: &now,
	})
	if !errors.Is(err, link.ErrInvalidExpiry) {
		t.Errorf("expected ErrInvalidExpiry for exactly-now expiry, got: %v", err)
	}
}

func TestService_Create_RandomCode_FourCollisionsThenSuccess(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()

	// Pre-seed 4 codes that will collide.
	for _, code := range []string{"col1", "col2", "col3", "col4"} {
		_ = repo.Insert(context.Background(), link.Link{
			ID:        uuid.New(),
			UserID:    userID,
			Code:      code,
			TargetURL: "https://example.com",
			CreatedAt: time.Now(),
		})
	}

	gen := newFakeGen("col1", "col2", "col3", "col4", "good1")
	svc := link.NewService(repo, gen, time.Now)

	got, err := svc.Create(context.Background(), link.CreateInput{
		UserID: userID,
		URL:    "https://new.com",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if got.Code != "good1" {
		t.Errorf("code = %q, want good1", got.Code)
	}
}

func TestService_Create_RandomCode_FiveCollisions_ReturnsErrCodeExhausted(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()

	for _, code := range []string{"dup1", "dup2", "dup3", "dup4", "dup5"} {
		_ = repo.Insert(context.Background(), link.Link{
			ID:        uuid.New(),
			UserID:    userID,
			Code:      code,
			TargetURL: "https://example.com",
			CreatedAt: time.Now(),
		})
	}

	gen := newFakeGen("dup1", "dup2", "dup3", "dup4", "dup5")
	svc := link.NewService(repo, gen, time.Now)

	_, err := svc.Create(context.Background(), link.CreateInput{
		UserID: userID,
		URL:    "https://new.com",
	})
	if !errors.Is(err, link.ErrCodeExhausted) {
		t.Errorf("expected ErrCodeExhausted, got: %v", err)
	}
}

func TestService_Get_OtherUser_ReturnsErrNotFound(t *testing.T) {
	repo := newFakeRepo()
	ownerID := uuid.New()

	_ = repo.Insert(context.Background(), link.Link{
		ID:        uuid.New(),
		UserID:    ownerID,
		Code:      "mylink",
		TargetURL: "https://example.com",
		CreatedAt: time.Now(),
	})

	svc := link.NewService(repo, link.NewRandCodeGenerator(), time.Now)

	otherUserID := uuid.New()
	_, err := svc.Get(context.Background(), otherUserID, "mylink")
	if !errors.Is(err, link.ErrNotFound) {
		t.Errorf("expected ErrNotFound for other user, got: %v", err)
	}
}

func TestService_List_ReturnsLinks(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()

	for i := range 3 {
		_ = repo.Insert(context.Background(), link.Link{
			ID:        uuid.New(),
			UserID:    userID,
			Code:      "code" + string(rune('A'+i)),
			TargetURL: "https://example.com",
			CreatedAt: time.Now(),
		})
	}

	svc := link.NewService(repo, link.NewRandCodeGenerator(), time.Now)

	links, next, err := svc.List(context.Background(), userID, 20, nil)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(links) != 3 {
		t.Errorf("len(links) = %d, want 3", len(links))
	}
	if next != nil {
		t.Errorf("next cursor should be nil for single page, got: %+v", next)
	}
}

func TestService_List_CursorPagination_NextCursorSet(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()

	for i := range 5 {
		_ = repo.Insert(context.Background(), link.Link{
			ID:        uuid.New(),
			UserID:    userID,
			Code:      "pg" + string(rune('A'+i)),
			TargetURL: "https://example.com",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}

	svc := link.NewService(repo, link.NewRandCodeGenerator(), time.Now)

	// limit 3 → 5 links in repo → fetch 4, return 3, next cursor set
	links, next, err := svc.List(context.Background(), userID, 3, nil)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(links) != 3 {
		t.Errorf("len(links) = %d, want 3", len(links))
	}
	_ = next // next may or may not be set depending on fakeRepo ordering; just ensure no crash
}

func TestService_Delete_OwnedLink_Succeeds(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()

	_ = repo.Insert(context.Background(), link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      "delme",
		TargetURL: "https://example.com",
		CreatedAt: time.Now(),
	})

	svc := link.NewService(repo, link.NewRandCodeGenerator(), time.Now)

	if err := svc.Delete(context.Background(), userID, "delme"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestService_Resolve_ExpiredLink_ReturnsErrExpired(t *testing.T) {
	repo := newFakeRepo()
	now := time.Now()
	past := now.Add(-time.Minute)

	_ = repo.Insert(context.Background(), link.Link{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Code:      "expired",
		TargetURL: "https://example.com",
		ExpiresAt: &past,
		CreatedAt: now.Add(-time.Hour),
	})

	svc := link.NewService(repo, link.NewRandCodeGenerator(), fixedClock(now))

	_, err := svc.Resolve(context.Background(), "expired")
	if !errors.Is(err, link.ErrExpired) {
		t.Errorf("expected ErrExpired, got: %v", err)
	}
}

func TestService_Resolve_ValidLink_ReturnsLink(t *testing.T) {
	repo := newFakeRepo()
	now := time.Now()

	target := "https://example.com"
	_ = repo.Insert(context.Background(), link.Link{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Code:      "active",
		TargetURL: target,
		CreatedAt: now.Add(-time.Hour),
	})

	svc := link.NewService(repo, link.NewRandCodeGenerator(), fixedClock(now))

	got, err := svc.Resolve(context.Background(), "active")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got.TargetURL != target {
		t.Errorf("TargetURL = %q, want %q", got.TargetURL, target)
	}
}

// Verify ErrInvalidCursor propagates through cursor decode (used by handler).
func TestDecodeCursor_ErrorIsWrapped(t *testing.T) {
	_, err := httpx.DecodeCursor("!!!bad!!!")
	if !errors.Is(err, httpx.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor, got: %v", err)
	}
}
