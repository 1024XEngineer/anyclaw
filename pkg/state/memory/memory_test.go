package memory

import (
	"strings"
	"testing"
)

func TestRandomIDUsesExpectedCharsetAndAvoidsDuplicates(t *testing.T) {
	t.Parallel()

	const (
		length = 8
		total  = 1024
		chars  = "abcdefghijklmnopqrstuvwxyz0123456789"
	)

	seen := make(map[string]struct{}, total)
	for i := 0; i < total; i++ {
		id, err := randomID(length)
		if err != nil {
			t.Fatalf("randomID(%d): %v", length, err)
		}
		if len(id) != length {
			t.Fatalf("expected ID length %d, got %d (%q)", length, len(id), id)
		}
		for _, ch := range id {
			if !strings.ContainsRune(chars, ch) {
				t.Fatalf("unexpected character %q in ID %q", ch, id)
			}
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}
