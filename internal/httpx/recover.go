package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover returns middleware that catches panics, logs the stack trace,
// and returns an RFC 9457 500 problem JSON response to the client.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID := GetRequestID(r.Context())
					logger.Error("panic recovered",
						"error", fmt.Sprint(rec),
						"stack", string(debug.Stack()),
						"request_id", reqID,
					)

					WriteProblem(w, r, http.StatusInternalServerError, "internal", "an unexpected error occurred")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
