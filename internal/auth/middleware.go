package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/httpx"
)

type contextKey int

const userContextKey contextKey = 1

// TokenVerifier defines the token validation method required by Authenticate middleware.
type TokenVerifier interface {
	VerifyToken(tokenStr string) (uuid.UUID, error)
}

// WithUser returns a new context containing the authenticated user ID.
func WithUser(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userContextKey, userID)
}

// UserFromContext extracts the authenticated user ID from context.
// Returns uuid.Nil and false if no user is present.
func UserFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userContextKey).(uuid.UUID)
	return id, ok
}

// Authenticate returns middleware that validates the Bearer token in the Authorization header.
// On success, it injects the user ID into the request context.
// On failure, it returns an RFC 9457 401 problem JSON response.
func Authenticate(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthorized", "invalid authorization header format")
				return
			}

			tokenStr := strings.TrimSpace(parts[1])
			userID, err := verifier.VerifyToken(tokenStr)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}

			ctx := WithUser(r.Context(), userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

