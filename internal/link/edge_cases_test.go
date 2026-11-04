package link_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kyyril/shortlink/internal/link"
)

// P5-2: Table-driven tests for URL input edge cases.
func TestService_ValidateTarget_EdgeCases(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)
	ctx := context.Background()
	userID := uuid.New()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Valid cases
		{name: "standard http", url: "http://example.com", wantErr: false},
		{name: "standard https", url: "https://example.com/path?foo=bar#hash", wantErr: false},
		{name: "mixed case scheme", url: "hTtPs://example.com/test", wantErr: false},
		{name: "subdomain and port", url: "https://sub.domain.org:8443/api/v1", wantErr: false},
		{name: "ipv4 host", url: "http://192.168.1.1:8080/page", wantErr: false},
		{name: "max length 2048 chars", url: "https://example.com/" + strings.Repeat("a", 2048-20), wantErr: false},

		// Rejected cases
		{name: "empty url", url: "", wantErr: true},
		{name: "exceeds 2048 chars", url: "https://example.com/" + strings.Repeat("a", 2048), wantErr: true},
		{name: "javascript scheme", url: "javascript:alert('xss')", wantErr: true},
		{name: "data scheme", url: "data:text/html,<script>alert(1)</script>", wantErr: true},
		{name: "ftp scheme", url: "ftp://ftp.example.com/file.zip", wantErr: true},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: true},
		{name: "no scheme", url: "example.com/path", wantErr: true},
		{name: "no host", url: "https://", wantErr: true},
		{name: "userinfo present", url: "https://admin:secret@example.com/login", wantErr: true},
		{name: "userinfo without pass", url: "https://admin@example.com", wantErr: true},
		{name: "control characters in url", url: "https://example.com/\n/bad", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, link.CreateInput{
				UserID: userID,
				URL:    tt.url,
			})
			if tt.wantErr {
				if !errors.Is(err, link.ErrInvalidURL) {
					t.Errorf("expected ErrInvalidURL for %q, got: %v", tt.url, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %q: %v", tt.url, err)
				}
			}
		})
	}
}

// P5-2: Table-driven tests for custom code validation and reserved codes.
func TestService_ValidateCustomCode_EdgeCases(t *testing.T) {
	svc := link.NewService(newFakeRepo(), link.NewRandCodeGenerator(), time.Now)
	ctx := context.Background()
	userID := uuid.New()

	tests := []struct {
		name       string
		customCode string
		wantErr    bool
	}{
		// Valid cases
		{name: "min 4 characters alphanumeric", customCode: "abcd", wantErr: false},
		{name: "max 32 characters alphanumeric", customCode: strings.Repeat("a", 32), wantErr: false},
		{name: "with dash and underscore", customCode: "my-custom_link12", wantErr: false},
		{name: "all numbers", customCode: "12345", wantErr: false},
		{name: "mixed case", customCode: "GoDoc2026", wantErr: false},

		// Rejected cases
		{name: "too short - 3 chars", customCode: "abc", wantErr: true},
		{name: "too long - 33 chars", customCode: strings.Repeat("a", 33), wantErr: true},
		{name: "contains space", customCode: "my link", wantErr: true},
		{name: "contains special characters", customCode: "code$name", wantErr: true},
		{name: "contains slash", customCode: "code/name", wantErr: true},
		{name: "contains unicode", customCode: "link☕", wantErr: true},

		// Reserved names
		{name: "reserved v1 exact", customCode: "v1", wantErr: true},
		{name: "reserved v1 uppercase", customCode: "V1", wantErr: true},
		{name: "reserved healthz exact", customCode: "healthz", wantErr: true},
		{name: "reserved healthz uppercase", customCode: "HEALTHZ", wantErr: true},
		{name: "reserved readyz exact", customCode: "readyz", wantErr: true},
		{name: "reserved readyz mixed case", customCode: "ReadyZ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, link.CreateInput{
				UserID:     userID,
				URL:        "https://example.com",
				CustomCode: tt.customCode,
			})
			if tt.wantErr {
				if !errors.Is(err, link.ErrInvalidCode) {
					t.Errorf("expected ErrInvalidCode for code %q, got: %v", tt.customCode, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for code %q: %v", tt.customCode, err)
				}
			}
		})
	}
}
