package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts all HTTP requests processed by method, path/pattern, and status code.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "shortlink",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed.",
		},
		[]string{"method", "status"},
	)

	// HTTPRequestDuration tracks request durations in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "shortlink",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency distributions in seconds.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		},
		[]string{"method", "status"},
	)

	// ClickDropsTotal counts dropped click events when the buffer is full.
	ClickDropsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "shortlink",
			Subsystem: "click",
			Name:      "drops_total",
			Help:      "Total number of click events dropped due to full buffer.",
		},
	)
)

// Metrics records duration and count metrics per HTTP request.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(rw.statusCode)

		HTTPRequestsTotal.WithLabelValues(r.Method, statusStr).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, statusStr).Observe(duration)
	})
}
