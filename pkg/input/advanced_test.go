package input

import "testing"

func TestChannelPairingListPairedReturnsEmptySlice(t *testing.T) {
	cp := NewChannelPairing()

	paired := cp.ListPaired()

	if paired == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(paired) != 0 {
		t.Fatalf("expected no paired devices, got %d", len(paired))
	}
}

func TestContactDirectoryListReturnsEmptySlice(t *testing.T) {
	cd := NewContactDirectory()

	contacts := cd.List("wechat")

	if contacts == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(contacts) != 0 {
		t.Fatalf("expected no contacts, got %d", len(contacts))
	}
}

func TestContactDirectorySearchReturnsEmptySlice(t *testing.T) {
	cd := NewContactDirectory()

	contacts := cd.Search("alice")

	if contacts == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(contacts) != 0 {
		t.Fatalf("expected no contacts, got %d", len(contacts))
	}
}
