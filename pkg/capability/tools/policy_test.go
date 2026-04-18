package tools

import (
	"path/filepath"
	"testing"
)

func TestPolicyEngineDeniesReadOutsideWorkingDir(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	policy := NewPolicyEngine(PolicyOptions{WorkingDir: workspace})

	if err := policy.CheckReadPath(outside); err == nil {
		t.Fatal("expected read outside working directory to be denied")
	}
}

func TestPolicyEngineAllowsConfiguredReadPath(t *testing.T) {
	workspace := t.TempDir()
	allowed := t.TempDir()
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:       workspace,
		AllowedReadPaths: []string{allowed},
	})

	if err := policy.CheckReadPath(allowed); err != nil {
		t.Fatalf("expected allowed read path, got %v", err)
	}
}

func TestPolicyEngineAllowsConfiguredWritePathEvenIfProtected(t *testing.T) {
	workspace := t.TempDir()
	desktopRoot := t.TempDir()
	target := filepath.Join(desktopRoot, "demo")
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:        workspace,
		ProtectedPaths:    []string{desktopRoot},
		AllowedWritePaths: []string{desktopRoot},
	})

	if err := policy.CheckWritePath(target); err != nil {
		t.Fatalf("expected explicit allowed write path to override protected path, got %v", err)
	}
}

func TestPolicyEngineDeniesBrowserUploadWithoutAllowedDomain(t *testing.T) {
	workspace := t.TempDir()
	policy := NewPolicyEngine(PolicyOptions{WorkingDir: workspace})

	if err := policy.CheckBrowserUpload(workspace, "https://example.com/upload"); err == nil {
		t.Fatal("expected browser upload to non-allowed domain to be denied")
	}
}

func TestPolicyEngineAllowsBrowserUploadToConfiguredDomain(t *testing.T) {
	workspace := t.TempDir()
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:           workspace,
		AllowedEgressDomains: []string{"example.com"},
	})

	if err := policy.CheckBrowserUpload(workspace, "https://example.com/upload"); err != nil {
		t.Fatalf("expected browser upload to allowed domain, got %v", err)
	}
}

func TestPolicyEngineAllowsLocalBrowserUploadWithoutEgressAllowlist(t *testing.T) {
	workspace := t.TempDir()
	policy := NewPolicyEngine(PolicyOptions{WorkingDir: workspace})

	if err := policy.CheckBrowserUpload(workspace, "http://127.0.0.1:3000/upload"); err != nil {
		t.Fatalf("expected local browser upload to be allowed, got %v", err)
	}
}

func TestPolicyEngineBlocksPluginNetOutWithoutAllowedDomain(t *testing.T) {
	policy := NewPolicyEngine(PolicyOptions{WorkingDir: t.TempDir()})

	if err := policy.ValidatePluginPermissions("demo-plugin", []string{"tool:exec", "net:out"}); err == nil {
		t.Fatal("expected net:out plugin permission to be denied without egress allowlist")
	}
}
