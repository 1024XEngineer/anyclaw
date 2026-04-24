package sqlite

import "testing"

func TestSidecarDirFileDB(t *testing.T) {
	db := &DB{cfg: Config{DSN: `C:\tmp\anyclaw.db`}}

	got := db.SidecarDir("vec")
	want := `C:\tmp\anyclaw.vec`
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSidecarDirInMemoryDB(t *testing.T) {
	db := &DB{cfg: Config{DSN: ":memory:"}}

	if got := db.SidecarDir("vec"); got != "" {
		t.Fatalf("expected empty sidecar dir for in-memory db, got %q", got)
	}
}
