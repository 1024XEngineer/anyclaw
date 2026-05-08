package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLifecycleUninstallRemovesReceiptBindingsAndInstallDir(t *testing.T) {
	store := NewStore(t.TempDir())
	installedPath := filepath.Join(store.InstalledDir(), "skill", "cloud-skill-release-notes", "1-0-0")
	if err := os.MkdirAll(installedPath, 0o755); err != nil {
		t.Fatal(err)
	}
	receipt := &InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        SourceCloud,
		InstalledPath: installedPath,
		InstalledBy:   "user",
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	binding, err := store.CreateBinding(BindingRequest{
		ArtifactID: receipt.ArtifactID,
		TargetType: TargetRuntimeGlobal,
		TargetID:   "",
		ReceiptID:  receipt.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewLifecycleService(store).Uninstall(UninstallRequest{ArtifactID: receipt.ArtifactID, Actor: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ReceiptID != receipt.ID || len(result.RemovedBindings) != 1 || result.RemovedBindings[0] != binding.ID {
		t.Fatalf("unexpected uninstall result: %#v", result)
	}
	if _, err := os.Stat(installedPath); !os.IsNotExist(err) {
		t.Fatalf("installed path still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := store.GetReceipt(receipt.ID); err != ErrArtifactNotFound {
		t.Fatalf("receipt err = %v, want ErrArtifactNotFound", err)
	}
	bindings, err := store.ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 0 {
		t.Fatalf("bindings = %#v, want empty", bindings.Items)
	}
	auditData, err := os.ReadFile(store.AuditPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(auditData), "market.uninstall.succeeded") {
		t.Fatalf("audit missing uninstall event: %s", string(auditData))
	}
}
