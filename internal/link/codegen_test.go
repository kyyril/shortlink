package link_test

import (
	"regexp"
	"testing"

	"github.com/kyyril/shortlink/internal/link"
)

func TestRandCodeGenerator_Length(t *testing.T) {
	g := link.NewRandCodeGenerator()

	code, err := g.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if len(code) != 7 {
		t.Errorf("code length = %d, want 7", len(code))
	}
}

func TestRandCodeGenerator_Alphabet(t *testing.T) {
	g := link.NewRandCodeGenerator()
	re := regexp.MustCompile(`^[A-Za-z0-9]{7}$`)

	for i := range 20 {
		code, err := g.New()
		if err != nil {
			t.Fatalf("iteration %d: New() error: %v", i, err)
		}

		if !re.MatchString(code) {
			t.Errorf("iteration %d: code %q contains characters outside [A-Za-z0-9]", i, code)
		}
	}
}

func TestRandCodeGenerator_Uniqueness(t *testing.T) {
	g := link.NewRandCodeGenerator()
	seen := make(map[string]bool)

	for i := range 100 {
		code, err := g.New()
		if err != nil {
			t.Fatalf("iteration %d: New() error: %v", i, err)
		}

		seen[code] = true
	}

	// 100 calls should produce at least 95 distinct codes (allowing extremely unlikely collisions).
	if len(seen) < 95 {
		t.Errorf("too many collisions: only %d unique codes from 100 calls", len(seen))
	}
}
