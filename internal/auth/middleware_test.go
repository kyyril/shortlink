package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/auth"
	"github.com/kyyril/shortlink/internal/httpx"
)

type fakeVerifier struct {
	verifyFunc func(tokenStr string) (uuid.UUID, error)
}

func (f *fakeVerifier) VerifyToken(tokenStr string) (uuid.UUID, error) {
	if f.verifyFunc != nil {
		return f.verifyFunc(tokenStr)
	}
	return uuid.Nil, auth.ErrTokenInvalid
}

// AC-7: Given no token or an invalid token, when I call a protected endpoint,
// then I get 401.
func TestAuthenticate_AC7(t *testing.T) {
	expectedUserID := uuid.New()

	tests := []struct {
		name          string
		authHeader    string
		verifier      auth.TokenVerifier
		wantStatus    int
		wantProblem   bool
		wantUserIDSet bool
	}{
		{
			name:       "MissingHeader_Returns401",
			authHeader: "",
			verifier: &fakeVerifier{
				verifyFunc: func(tokenStr string) (uuid.UUID, error) {
					return expectedUserID, nil
				},
			},
			wantStatus:  http.StatusUnauthorized,
			wantProblem: true,
		},
		{
			name:       "MalformedHeader_NoBearerPrefix_Returns401",
			authHeader: "Basic dXNlcjpwYXNz",
			verifier: &fakeVerifier{
				verifyFunc: func(tokenStr string) (uuid.UUID, error) {
					return expectedUserID, nil
				},
			},
			wantStatus:  http.StatusUnauthorized,
			wantProblem: true,
		},
		{
			name:       "MalformedHeader_EmptyToken_Returns401",
			authHeader: "Bearer ",
			verifier: &fakeVerifier{
				verifyFunc: func(tokenStr string) (uuid.UUID, error) {
					return expectedUserID, nil
				},
			},
			wantStatus:  http.StatusUnauthorized,
			wantProblem: true,
		},
		{
			name:       "InvalidToken_Returns401",
			authHeader: "Bearer invalid.jwt.token",
			verifier: &fakeVerifier{
				verifyFunc: func(tokenStr string) (uuid.UUID, error) {
					return uuid.Nil, auth.ErrTokenInvalid
				},
			},
			wantStatus:  http.StatusUnauthorized,
			wantProblem: true,
		},
		{
			name:       "ExpiredToken_Returns401",
			authHeader: "Bearer expired.jwt.token",
			verifier: &fakeVerifier{
				verifyFunc: func(tokenStr string) (uuid.UUID, error) {
					return uuid.Nil, auth.ErrTokenExpired
				},
			},
			wantStatus:  http.StatusUnauthorized,
			wantProblem: true,
		},
		{
			name:       "ValidToken_StoresUserIDInContext_PassesToNext",
			authHeader: "Bearer valid.jwt.token",
			verifier: &fakeVerifier{
				verifyFunc: func(tokenStr string) (uuid.UUID, error) {
					if tokenStr == "valid.jwt.token" {
						return expectedUserID, nil
					}
					return uuid.Nil, auth.ErrTokenInvalid
				},
			},
			wantStatus:    http.StatusOK,
			wantProblem:   false,
			wantUserIDSet: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedUserID uuid.UUID
			var capturedPresent bool

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedUserID, capturedPresent = auth.UserFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			mw := auth.Authenticate(tc.verifier)(next)

			req := httptest.NewRequest(http.MethodGet, "/v1/links", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()

			mw.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantProblem {
				var p httpx.Problem
				if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
					t.Fatalf("failed to decode problem JSON: %v", err)
				}
				if p.Status != http.StatusUnauthorized {
					t.Errorf("problem status = %d, want 401", p.Status)
				}
				if p.Code != "unauthorized" {
					t.Errorf("problem code = %q, want 'unauthorized'", p.Code)
				}
			}

			if tc.wantUserIDSet {
				if !capturedPresent {
					t.Error("expected user ID in context, but was not found")
				}
				if capturedUserID != expectedUserID {
					t.Errorf("captured user ID = %v, want %v", capturedUserID, expectedUserID)
				}
			}
		})
	}
}
