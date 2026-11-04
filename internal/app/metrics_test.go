package app_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyyril/shortlink/internal/app"
)

// P6-3: Scrape /metrics endpoint and verify required Prometheus metric names exist.
func TestMetrics_Scrape_ContainsExpectedMetricNames(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	application := app.New(cfg, logger)

	// Fire one request to /healthz so request_duration and requests_total counters observe data
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHealth := httptest.NewRecorder()
	application.Routes().ServeHTTP(recHealth, reqHealth)

	// Scrape /metrics
	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recMetrics := httptest.NewRecorder()
	application.Routes().ServeHTTP(recMetrics, reqMetrics)

	if recMetrics.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", recMetrics.Code)
	}

	body := recMetrics.Body.String()

	expectedMetrics := []string{
		"shortlink_http_requests_total",
		"shortlink_http_request_duration_seconds",
		"shortlink_click_drops_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected /metrics output to contain %q, but was missing", metric)
		}
	}
}
