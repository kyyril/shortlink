package link

import (
	"crypto/rand"
	"fmt"
)

const (
	codeLength = 7
	alphabet   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// CodeGenerator produces random short codes.
type CodeGenerator interface {
	// New returns a fresh short code or an error if randomness is unavailable.
	New() (string, error)
}

// RandCodeGenerator produces 7-character base62 codes using crypto/rand.
type RandCodeGenerator struct{}

// NewRandCodeGenerator returns a ready-to-use RandCodeGenerator.
func NewRandCodeGenerator() *RandCodeGenerator {
	return &RandCodeGenerator{}
}

// New returns a 7-character base62 string drawn from crypto/rand.
func (g *RandCodeGenerator) New() (string, error) {
	buf := make([]byte, codeLength)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	code := make([]byte, codeLength)
	for i, b := range buf {
		code[i] = alphabet[int(b)%len(alphabet)]
	}

	return string(code), nil
}
