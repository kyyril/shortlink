package link_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/link"
)

type fakeIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]link.IdempotencyRecord
}

func newFakeIdempotencyStore() *fakeIdempotencyStore {
	return &fakeIdempotencyStore{
		records: make(map[string]link.IdempotencyRecord),
	}
}

func (s *fakeIdempotencyStore) Get(_ context.Context, key string, userID uuid.UUID) (*link.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapKey := fmt.Sprintf("%s:%s", userID.String(), key)
	rec, ok := s.records[mapKey]
	if !ok {
		return nil, nil
	}
	return &rec, nil
}

func (s *fakeIdempotencyStore) Save(_ context.Context, rec link.IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapKey := fmt.Sprintf("%s:%s", rec.UserID.String(), rec.Key)
	s.records[mapKey] = rec
	return nil
}

// P5-3: Same key + same body -> original response (replay)
func TestHandler_Idempotency_SameKeySameBody_ReplaysResponse(t *testing.T) {
	userID := uuid.New()
	idemStore := newFakeIdempotencyStore()

	linkID := uuid.New()
	svc := &fakeService{
		createFn: func(_ context.Context, in link.CreateInput) (link.Link, error) {
			return link.Link{
				ID:        linkID,
				UserID:    userID,
				Code:      "idem001",
				TargetURL: in.URL,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly", link.WithIdempotencyStore(idemStore))
	payload := map[string]string{"url": "https://example.com/unique"}
	b, _ := json.Marshal(payload)

	// First Request
	req1 := httptest.NewRequest(http.MethodPost, "/v1/links", bytes.NewReader(b))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "key-12345")
	w1 := httptest.NewRecorder()
	h.Create(w1, withUserCtx(req1, userID))

	if w1.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want 201", w1.Code)
	}

	// Second Request with SAME key and SAME body
	req2 := httptest.NewRequest(http.MethodPost, "/v1/links", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "key-12345")
	w2 := httptest.NewRecorder()
	h.Create(w2, withUserCtx(req2, userID))

	if w2.Code != http.StatusCreated {
		t.Fatalf("second request status = %d, want 201", w2.Code)
	}

	if w1.Body.String() != w2.Body.String() {
		t.Errorf("expected replayed body to match: %s != %s", w1.Body.String(), w2.Body.String())
	}
}

// P5-3: Same key + different body -> 422 idempotency_conflict
func TestHandler_Idempotency_SameKeyDifferentBody_Returns422(t *testing.T) {
	userID := uuid.New()
	idemStore := newFakeIdempotencyStore()

	svc := &fakeService{
		createFn: func(_ context.Context, in link.CreateInput) (link.Link, error) {
			return link.Link{
				ID:        uuid.New(),
				UserID:    userID,
				Code:      "code123",
				TargetURL: in.URL,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly", link.WithIdempotencyStore(idemStore))

	// First Request
	b1, _ := json.Marshal(map[string]string{"url": "https://example.com/first"})
	req1 := httptest.NewRequest(http.MethodPost, "/v1/links", bytes.NewReader(b1))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "conflict-key")
	w1 := httptest.NewRecorder()
	h.Create(w1, withUserCtx(req1, userID))

	if w1.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want 201", w1.Code)
	}

	// Second Request with SAME key and DIFFERENT body
	b2, _ := json.Marshal(map[string]string{"url": "https://example.com/different"})
	req2 := httptest.NewRequest(http.MethodPost, "/v1/links", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "conflict-key")
	w2 := httptest.NewRecorder()
	h.Create(w2, withUserCtx(req2, userID))

	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w2.Code)
	}

	var prob map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &prob)
	if prob["code"] != "idempotency_conflict" {
		t.Errorf("problem code = %v, want idempotency_conflict", prob["code"])
	}
}

// P5-3: Concurrent duplicate requests with the same key
func TestHandler_Idempotency_ConcurrentDuplicates(t *testing.T) {
	userID := uuid.New()
	idemStore := newFakeIdempotencyStore()

	var createCalls int
	var mu sync.Mutex
	svc := &fakeService{
		createFn: func(_ context.Context, in link.CreateInput) (link.Link, error) {
			mu.Lock()
			createCalls++
			mu.Unlock()
			return link.Link{
				ID:        uuid.New(),
				UserID:    userID,
				Code:      "conc123",
				TargetURL: in.URL,
				CreatedAt: time.Now(),
			}, nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly", link.WithIdempotencyStore(idemStore))
	b, _ := json.Marshal(map[string]string{"url": "https://example.com/concurrent"})

	var wg sync.WaitGroup
	results := make([]int, 5)

	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/v1/links", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "concurrent-key")
			w := httptest.NewRecorder()
			h.Create(w, withUserCtx(req, userID))
			results[idx] = w.Code
		}(i)
	}

	wg.Wait()

	for _, code := range results {
		if code != http.StatusCreated {
			t.Errorf("expected 201 for all concurrent idempotent requests, got %d", code)
		}
	}
}
