package app_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/kyyril/shortlink/internal/app"
	"github.com/kyyril/shortlink/internal/config"
)

func TestApp_GracefulShutdown_SlowRequestCompletes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen random port: %v", err)
	}

	cfg := &config.Config{
		AppEnv:          "development",
		HTTPAddr:        ln.Addr().String(),
		BaseURL:         "http://" + ln.Addr().String(),
		DatabaseURL:     "postgres://user:pass@localhost:5432/db",
		JWTSecret:       "this-secret-is-at-least-32-bytes-long",
		JWTTTL:          15 * time.Minute,
		LogLevel:        "info",
		LogFormat:       "json",
		ShutdownTimeout: 5 * time.Second,
		RequestTimeout:  4 * time.Second,
		RateLimitRPS:    10,
		RateLimitBurst:  20,
		ClickBufferSize: 1024,
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	requestStarted := make(chan struct{})
	finishRequest := make(chan struct{})

	slowHandler := func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-finishRequest
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow response finished"))
	}

	application := app.New(cfg, logger,
		app.WithListener(ln),
		app.WithRoute("GET /slow", slowHandler),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- application.Run(ctx)
	}()

	// 1. Send the slow request
	var resp *http.Response
	var reqErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		client := &http.Client{Timeout: 10 * time.Second}
		resp, reqErr = client.Get("http://" + ln.Addr().String() + "/slow")
	}()

	// 2. Wait until the slow request has started processing
	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for slow request to start")
	}

	// 3. Trigger shutdown while the slow request is in-flight
	cancel()

	// 4. Release the slow request so it can finish
	close(finishRequest)

	// 5. Wait for request to complete
	wg.Wait()

	if reqErr != nil {
		t.Fatalf("slow request failed: %v", reqErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "slow response finished" {
		t.Errorf("body = %q, want 'slow response finished'", string(body))
	}

	// 6. Verify that app.Run exits cleanly
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("app.Run unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for app.Run to shut down")
	}
}
