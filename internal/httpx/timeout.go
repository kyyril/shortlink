package httpx

import (
	"context"
	"net/http"
	"time"
)

// Timeout returns middleware that wraps the request context with a timeout.
// If the timeout fires, any I/O or database query reading from r.Context() is cancelled.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
