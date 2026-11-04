//go:build integration

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kyyril/shortlink/internal/app"
	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/click"
	"github.com/kyyril/shortlink/internal/link"
	"github.com/kyyril/shortlink/internal/store"
)

// buildTestApp constructs a full application stack wired to the real database.
// It returns the app handler, recorder, and a cleanup function.
func buildTestApp(t *testing.T) (http.Handler, *click.Recorder, func()) {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://shortlink:shortlink@localhost:5433/shortlink?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := store.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	cfg := testConfig()
	cfg.BaseURL = "http://short.test"
	cfg.RequestTimeout = 10 * time.Second

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	userRepo := store.NewUserRepo(pool)
	authSvc := auth.NewService(userRepo, cfg.JWTSecret, cfg.JWTTTL, time.Now)
	authHandler := auth.NewHandler(authSvc)

	clickRepo := store.NewClickRepo(pool)
	recorder := click.NewRecorder(clickRepo, 1024, 10, 100*time.Millisecond, logger)
	recorder.Start()

	linkRepo := store.NewLinkRepo(pool)
	linkSvc := link.NewService(linkRepo, link.NewRandCodeGenerator(), time.Now)
	linkHandler := link.NewHandler(linkSvc, cfg.BaseURL, link.WithClickRecorder(recorder))

	statsSvc := click.NewService(clickRepo, linkSvc, time.Now)
	statsHandler := click.NewHandler(statsSvc)

	// Use a random port listener so tests don't conflict.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	application := app.New(
		cfg,
		logger,
		app.WithListener(ln),
		app.WithPinger(pool),
		app.WithAuthHandler(authHandler),
		app.WithLinkHandler(linkHandler, authSvc),
		app.WithStatsHandler(statsHandler),
		app.WithRecorder(recorder),
	)
	application.SetReady(true)

	cleanup := func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelClose()
		_ = recorder.Close(closeCtx)
		pool.Close()
	}

	return application.Routes(), recorder, cleanup
}

// doJSON is a helper that sends a JSON request against the test handler and returns the response.
func doJSON(t *testing.T, handler http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// TestE2E_RegisterLoginCreateRedirectDelete is the black-box end-to-end test (AC-11, AC-12 scope).
// It covers: register → login → create link → redirect → delete → 404.
func TestE2E_RegisterLoginCreateRedirectDelete(t *testing.T) {
	handler, _, cleanup := buildTestApp(t)
	defer cleanup()

	email := fmt.Sprintf("e2e_%s@example.com", strings.Replace(time.Now().Format("150405.000"), ".", "", 1))
	password := "securepassword123"

	// 1. Register
	rec := doJSON(t, handler, http.MethodPost, "/v1/auth/register", map[string]string{
		"email":    email,
		"password": password,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want 201; body = %s", rec.Code, rec.Body)
	}

	// 2. Login
	rec = doJSON(t, handler, http.MethodPost, "/v1/auth/login", map[string]string{
		"email":    email,
		"password": password,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("login: unmarshal: %v", err)
	}
	if loginResp.AccessToken == "" {
		t.Fatal("login: access_token is empty")
	}

	bearer := map[string]string{"Authorization": "Bearer " + loginResp.AccessToken}

	// 3. Create link
	targetURL := "https://go.dev/doc/effective_go"
	rec = doJSON(t, handler, http.MethodPost, "/v1/links", map[string]string{
		"url": targetURL,
	}, bearer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create link: status = %d, want 201; body = %s", rec.Code, rec.Body)
	}

	var createResp struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("create link: unmarshal: %v", err)
	}
	if len(createResp.Code) != 7 {
		t.Errorf("create link: code length = %d, want 7", len(createResp.Code))
	}
	if createResp.ShortURL == "" {
		t.Error("create link: short_url is empty")
	}

	code := createResp.Code

	// 4. Follow redirect (AC-11): GET /{code} returns 302 with correct Location.
	rec = doJSON(t, handler, http.MethodGet, "/"+code, nil, nil)
	if rec.Code != http.StatusFound {
		t.Errorf("redirect: status = %d, want 302; body = %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != targetURL {
		t.Errorf("redirect: Location = %q, want %q", loc, targetURL)
	}

	// 5. Get own link
	rec = doJSON(t, handler, http.MethodGet, "/v1/links/"+code, nil, bearer)
	if rec.Code != http.StatusOK {
		t.Errorf("get link: status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	// 6. Delete link
	rec = doJSON(t, handler, http.MethodDelete, "/v1/links/"+code, nil, bearer)
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete link: status = %d, want 204; body = %s", rec.Code, rec.Body)
	}

	// 7. After delete: redirect returns 404.
	rec = doJSON(t, handler, http.MethodGet, "/"+code, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("redirect after delete: status = %d, want 404", rec.Code)
	}
}

// TestE2E_ExpiredLink_Returns410 verifies AC-12: expired link returns 410.
func TestE2E_ExpiredLink_Returns410(t *testing.T) {
	handler, _, cleanup := buildTestApp(t)
	defer cleanup()

	email := fmt.Sprintf("e2e_exp_%s@example.com", strings.Replace(time.Now().Format("150405.000"), ".", "", 1))
	password := "securepassword123"

	// Register + Login
	doJSON(t, handler, http.MethodPost, "/v1/auth/register", map[string]string{
		"email": email, "password": password,
	}, nil)

	rec := doJSON(t, handler, http.MethodPost, "/v1/auth/login", map[string]string{
		"email": email, "password": password,
	}, nil)

	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("login unmarshal: %v", err)
	}

	bearer := map[string]string{"Authorization": "Bearer " + loginResp.AccessToken}

	// Create link that expires 1 second in the future.
	expiresAt := time.Now().UTC().Add(2 * time.Second)
	body := map[string]any{
		"url":        "https://example.com",
		"expires_at": expiresAt.Format(time.RFC3339),
	}
	rec = doJSON(t, handler, http.MethodPost, "/v1/links", body, bearer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create expiring link: status = %d; body = %s", rec.Code, rec.Body)
	}

	var createResp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("create expiring link: unmarshal: %v", err)
	}

	code := createResp.Code

	// Redirect while still active — expect 302.
	rec = doJSON(t, handler, http.MethodGet, "/"+code, nil, nil)
	if rec.Code != http.StatusFound {
		t.Errorf("active link: status = %d, want 302", rec.Code)
	}

	// Wait for expiry.
	time.Sleep(3 * time.Second)

	// Redirect after expiry — expect 410 (AC-12).
	rec = doJSON(t, handler, http.MethodGet, "/"+code, nil, nil)
	if rec.Code != http.StatusGone {
		t.Errorf("expired link: status = %d, want 410", rec.Code)
	}

	var prob struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &prob); err != nil {
		t.Fatalf("expired link: unmarshal problem: %v", err)
	}
	if prob.Code != "link_expired" {
		t.Errorf("expired link: problem code = %q, want link_expired", prob.Code)
	}
}

// TestE2E_ClickRecorderAndStats verifies AC-13: 3 redirects produce 3 clicks in stats,
// and another user receives 404 when querying stats.
func TestE2E_ClickRecorderAndStats(t *testing.T) {
	handler, recorder, cleanup := buildTestApp(t)
	defer cleanup()

	email := fmt.Sprintf("e2e_stats_%s@example.com", strings.Replace(time.Now().Format("150405.000"), ".", "", 1))
	password := "securepassword123"

	// Register & Login Owner
	doJSON(t, handler, http.MethodPost, "/v1/auth/register", map[string]string{
		"email": email, "password": password,
	}, nil)

	rec := doJSON(t, handler, http.MethodPost, "/v1/auth/login", map[string]string{
		"email": email, "password": password,
	}, nil)

	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)
	ownerBearer := map[string]string{"Authorization": "Bearer " + loginResp.AccessToken}

	// Create Link
	rec = doJSON(t, handler, http.MethodPost, "/v1/links", map[string]string{
		"url": "https://example.com/target",
	}, ownerBearer)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create link failed: %s", rec.Body)
	}

	var linkResp struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &linkResp)
	code := linkResp.Code

	// Perform 3 redirects
	for range 3 {
		rec = doJSON(t, handler, http.MethodGet, "/"+code, nil, nil)
		if rec.Code != http.StatusFound {
			t.Fatalf("redirect returned %d, want 302", rec.Code)
		}
	}

	// Flush recorder by closing it or giving it time to drain
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelClose()
	if err := recorder.Close(closeCtx); err != nil {
		t.Fatalf("recorder close: %v", err)
	}

	// Read stats as owner (AC-13)
	rec = doJSON(t, handler, http.MethodGet, "/v1/links/"+code+"/stats", nil, ownerBearer)
	if rec.Code != http.StatusOK {
		t.Fatalf("get stats: status = %d, want 200; body = %s", rec.Code, rec.Body)
	}

	var statsResp struct {
		TotalClicks int64 `json:"total_clicks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &statsResp); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if statsResp.TotalClicks != 3 {
		t.Errorf("total clicks = %d, want 3", statsResp.TotalClicks)
	}

	// Register & Login another user
	otherEmail := fmt.Sprintf("other_%s@example.com", strings.Replace(time.Now().Format("150405.000"), ".", "", 1))
	doJSON(t, handler, http.MethodPost, "/v1/auth/register", map[string]string{
		"email": otherEmail, "password": password,
	}, nil)

	rec = doJSON(t, handler, http.MethodPost, "/v1/auth/login", map[string]string{
		"email": otherEmail, "password": password,
	}, nil)

	var otherLoginResp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &otherLoginResp)
	otherBearer := map[string]string{"Authorization": "Bearer " + otherLoginResp.AccessToken}

	// Another user querying stats on this link gets 404
	rec = doJSON(t, handler, http.MethodGet, "/v1/links/"+code+"/stats", nil, otherBearer)
	if rec.Code != http.StatusNotFound {
		t.Errorf("other user stats: status = %d, want 404", rec.Code)
	}
}
