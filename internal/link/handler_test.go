package link_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/httpx"
	"github.com/kyyril/shortlink/internal/link"
)

// --- fake service for handler tests ---

type fakeService struct {
	createFn  func(ctx context.Context, in link.CreateInput) (link.Link, error)
	getFn     func(ctx context.Context, userID uuid.UUID, code string) (link.Link, error)
	listFn    func(ctx context.Context, userID uuid.UUID, limit int, cursor *httpx.Cursor) ([]link.Link, *httpx.Cursor, error)
	deleteFn  func(ctx context.Context, userID uuid.UUID, code string) error
	resolveFn func(ctx context.Context, code string) (link.Link, error)
}

func (s *fakeService) Create(ctx context.Context, in link.CreateInput) (link.Link, error) {
	return s.createFn(ctx, in)
}

func (s *fakeService) Get(ctx context.Context, userID uuid.UUID, code string) (link.Link, error) {
	return s.getFn(ctx, userID, code)
}

func (s *fakeService) List(ctx context.Context, userID uuid.UUID, limit int, cursor *httpx.Cursor) ([]link.Link, *httpx.Cursor, error) {
	return s.listFn(ctx, userID, limit, cursor)
}

func (s *fakeService) Delete(ctx context.Context, userID uuid.UUID, code string) error {
	return s.deleteFn(ctx, userID, code)
}

func (s *fakeService) Resolve(ctx context.Context, code string) (link.Link, error) {
	return s.resolveFn(ctx, code)
}

// withUserCtx returns a request with the given userID injected into context.
func withUserCtx(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := auth.WithUser(r.Context(), userID)
	return r.WithContext(ctx)
}

// newJSONRequest builds a POST request with a JSON body.
func newJSONRequest(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestHandler_Create_ValidURL_Returns201WithCodeAndShortURL(t *testing.T) {
	userID := uuid.New()
	expectedLink := link.Link{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      "abc1234",
		TargetURL: "https://example.com",
		CreatedAt: time.Now(),
	}

	svc := &fakeService{
		createFn: func(_ context.Context, _ link.CreateInput) (link.Link, error) {
			return expectedLink, nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	w := httptest.NewRecorder()
	r := withUserCtx(newJSONRequest(http.MethodPost, "/v1/links", map[string]string{"url": "https://example.com"}), userID)

	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp["code"] != "abc1234" {
		t.Errorf("code = %v, want abc1234", resp["code"])
	}
	if resp["short_url"] != "https://short.ly/abc1234" {
		t.Errorf("short_url = %v, want https://short.ly/abc1234", resp["short_url"])
	}
	// Must never expose user_id directly in link response as ownership info
	if _, hasUserID := resp["user_id"]; hasUserID {
		t.Error("response must not contain user_id field")
	}
}

func TestHandler_Create_InvalidURL_Returns422InvalidURL(t *testing.T) {
	svc := &fakeService{
		createFn: func(_ context.Context, _ link.CreateInput) (link.Link, error) {
			return link.Link{}, link.ErrInvalidURL
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	w := httptest.NewRecorder()
	r := withUserCtx(newJSONRequest(http.MethodPost, "/v1/links", map[string]string{"url": "javascript:alert(1)"}), uuid.New())

	h.Create(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["code"] != "invalid_url" {
		t.Errorf("error code = %v, want invalid_url", resp["code"])
	}
}

func TestHandler_Create_CodeTaken_Returns409CodeTaken(t *testing.T) {
	svc := &fakeService{
		createFn: func(_ context.Context, _ link.CreateInput) (link.Link, error) {
			return link.Link{}, link.ErrCodeTaken
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	w := httptest.NewRecorder()
	r := withUserCtx(newJSONRequest(http.MethodPost, "/v1/links", map[string]any{
		"url":         "https://example.com",
		"custom_code": "taken",
	}), uuid.New())

	h.Create(w, r)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["code"] != "code_taken" {
		t.Errorf("error code = %v, want code_taken", resp["code"])
	}
}

func TestHandler_Get_OtherUserCode_Returns404(t *testing.T) {
	svc := &fakeService{
		getFn: func(_ context.Context, _ uuid.UUID, _ string) (link.Link, error) {
			return link.Link{}, link.ErrNotFound
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/links/{code}", h.Get)

	w := httptest.NewRecorder()
	r := withUserCtx(httptest.NewRequest(http.MethodGet, "/v1/links/othercode", nil), uuid.New())

	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_Delete_Returns204(t *testing.T) {
	svc := &fakeService{
		deleteFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			return nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /v1/links/{code}", h.Delete)

	w := httptest.NewRecorder()
	r := withUserCtx(httptest.NewRequest(http.MethodDelete, "/v1/links/mycode", nil), uuid.New())

	mux.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestHandler_List_TamperedCursor_Returns400(t *testing.T) {
	svc := &fakeService{}

	h := link.NewHandler(svc, "https://short.ly")
	w := httptest.NewRecorder()
	r := withUserCtx(httptest.NewRequest(http.MethodGet, "/v1/links?cursor=tampered!!!", nil), uuid.New())

	h.List(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["code"] != "invalid_cursor" {
		t.Errorf("error code = %v, want invalid_cursor", resp["code"])
	}
}

func TestHandler_List_ValidFirstPage_Returns200(t *testing.T) {
	userID := uuid.New()
	now := time.Now()

	svc := &fakeService{
		listFn: func(_ context.Context, _ uuid.UUID, _ int, _ *httpx.Cursor) ([]link.Link, *httpx.Cursor, error) {
			return []link.Link{
				{ID: uuid.New(), UserID: userID, Code: "abc", TargetURL: "https://a.com", CreatedAt: now},
				{ID: uuid.New(), UserID: userID, Code: "def", TargetURL: "https://b.com", CreatedAt: now.Add(-time.Second)},
			}, nil, nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	w := httptest.NewRecorder()
	r := withUserCtx(httptest.NewRequest(http.MethodGet, "/v1/links", nil), userID)

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	links, ok := resp["links"].([]any)
	if !ok {
		t.Fatalf("links field missing or wrong type")
	}
	if len(links) != 2 {
		t.Errorf("links count = %d, want 2", len(links))
	}

	if _, hasNext := resp["next_cursor"]; hasNext {
		t.Error("next_cursor should be absent on last page")
	}
}

// TestHandler_List_Pagination_AC9 is an integration-style unit test verifying that
// pagination through 45 links with limit=20 visits all links exactly once.
func TestHandler_List_Pagination_AC9(t *testing.T) {
	userID := uuid.New()
	now := time.Now()

	// Build 45 fake links.
	allLinks := make([]link.Link, 45)
	for i := range 45 {
		allLinks[i] = link.Link{
			ID:        uuid.New(),
			UserID:    userID,
			Code:      fmt.Sprintf("link%02d", i),
			TargetURL: "https://example.com",
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	// fakeListFn simulates keyset pagination over allLinks (newest first).
	// It slices by cursor position and returns limit+1 to signal there is a next page.
	fakeListFn := func(_ context.Context, _ uuid.UUID, limit int, cursor *httpx.Cursor) ([]link.Link, *httpx.Cursor, error) {
		// Sort: newest first (index 44 = most recent).
		sorted := make([]link.Link, len(allLinks))
		for i, l := range allLinks {
			sorted[len(allLinks)-1-i] = l
		}

		start := 0
		if cursor != nil {
			for i, l := range sorted {
				if l.ID == cursor.ID {
					start = i + 1
					break
				}
			}
		}

		end := start + limit + 1
		if end > len(sorted) {
			end = len(sorted)
		}
		page := sorted[start:end]

		var next *httpx.Cursor
		if len(page) > limit {
			page = page[:limit]
			last := page[len(page)-1]
			next = &httpx.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		}

		return page, next, nil
	}

	svc := &fakeService{listFn: fakeListFn}
	h := link.NewHandler(svc, "https://short.ly")

	seen := map[string]int{}
	var cursorStr string

	for page := 0; ; page++ {
		url := "/v1/links?limit=20"
		if cursorStr != "" {
			url += "&cursor=" + cursorStr
		}

		w := httptest.NewRecorder()
		r := withUserCtx(httptest.NewRequest(http.MethodGet, url, nil), userID)
		h.List(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("page %d: status = %d, want 200", page, w.Code)
		}

		var resp struct {
			Links []struct {
				Code string `json:"code"`
			} `json:"links"`
			NextCursor *string `json:"next_cursor,omitempty"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("page %d: unmarshal: %v", page, err)
		}

		for _, l := range resp.Links {
			seen[l.Code]++
		}

		if resp.NextCursor == nil {
			break
		}
		cursorStr = *resp.NextCursor

		if page > 5 {
			t.Fatal("too many pages, likely infinite loop")
		}
	}

	if len(seen) != 45 {
		t.Errorf("saw %d unique links, want 45", len(seen))
	}
	for code, count := range seen {
		if count != 1 {
			t.Errorf("link %q seen %d times, want 1", code, count)
		}
	}
}

func TestHandler_Redirect_ValidCode_Returns302(t *testing.T) {
	target := "https://example.com/long-url"
	svc := &fakeService{
		resolveFn: func(_ context.Context, _ string) (link.Link, error) {
			return link.Link{
				Code:      "abc1234",
				TargetURL: target,
			}, nil
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", h.Redirect)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/abc1234", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, "https://example.com") {
		t.Errorf("Location = %q, want prefix https://example.com", loc)
	}
}

func TestHandler_Redirect_ExpiredLink_Returns410(t *testing.T) {
	svc := &fakeService{
		resolveFn: func(_ context.Context, _ string) (link.Link, error) {
			return link.Link{}, link.ErrExpired
		},
	}

	h := link.NewHandler(svc, "https://short.ly")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", h.Redirect)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/expired", nil)
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusGone {
		t.Errorf("status = %d, want %d", w.Code, http.StatusGone)
	}
}
