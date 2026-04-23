package qmd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTableNotFound  = errors.New("qmd: table not found")
	ErrRecordNotFound = errors.New("qmd: record not found")
	ErrInvalidRequest = errors.New("qmd: invalid request")
)

func newWALID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomHex(8))
}

func newRecordID() string {
	return randomHex(16)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
