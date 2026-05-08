package marketplace

import (
	"testing"
	"time"
)

func TestStoreBindingsDeriveArtifactStatus(t *testing.T) {
	store := NewStore(t.TempDir())
	receipt := &InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        SourceCloud,
		InstalledPath: "/tmp/release-notes",
		InstalledBy:   "user",
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}

	items := store.OverlayStatus([]Artifact{{
		ID:     "cloud.skill.release-notes",
		Kind:   ArtifactKindSkill,
		Source: SourceCloud,
		Status: StatusAvailable,
	}})
	if items[0].Status != StatusInstalled || !items[0].Installed || items[0].Bound || items[0].Active {
		t.Fatalf("expected installed-only status, got %#v", items[0])
	}

	binding, err := store.CreateBinding(BindingRequest{
		ArtifactID: receipt.ArtifactID,
		TargetType: TargetWorkspace,
		TargetID:   "workspace-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	items = store.OverlayStatus(items)
	if items[0].Status != StatusBound || !items[0].Bound || items[0].Active {
		t.Fatalf("expected bound status, got %#v", items[0])
	}

	if err := store.DeleteBinding(binding.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBinding(BindingRequest{
		ArtifactID: receipt.ArtifactID,
		TargetType: TargetMainAgent,
		TargetID:   "Main Agent",
	}); err != nil {
		t.Fatal(err)
	}
	items = store.OverlayStatus(items)
	if items[0].Status != StatusActive || !items[0].Active {
		t.Fatalf("expected active status for main_agent binding, got %#v", items[0])
	}
}
