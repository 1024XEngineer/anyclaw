package util

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a random hex identifier.
func NewID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
