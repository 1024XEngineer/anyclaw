package tools

import (
	"path/filepath"
	"strings"
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

func TestPolicyEngineDeniesProtectedPathInsideWorkingDir(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "private")
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:     workspace,
		ProtectedPaths: []string{protected},
	})

	if err := policy.CheckReadPath(filepath.Join(protected, "secret.txt")); err == nil {
		t.Fatal("expected protected path inside working directory to be denied")
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

func TestPrivacyEngineClassifyAndCommandChecks(t *testing.T) {
	engine := NewPrivacyEngine(PrivacyOptions{
		AllowedEgressDomains: []string{"example.com"},
		BlockedEgressDomains: []string{"blocked.com"},
	})

	if got := engine.ClassifyPath(`C:\Users\demo\.ssh\id_rsa`); got != PrivacyDomainKeys {
		t.Fatalf("expected SSH path to map to keys domain, got %v", got)
	}
	result := engine.CheckPath(`C:\Users\demo\Documents\Personal\notes.txt`)
	if result.IsAllowed || !result.RequiresApproval || result.Domain != PrivacyDomainDocuments {
		t.Fatalf("unexpected privacy check result: %#v", result)
	}
	if allowed, reason := engine.CheckCommand("rm -rf /tmp/demo"); allowed || !strings.Contains(reason, "dangerous command pattern") {
		t.Fatalf("expected dangerous command detection, got allowed=%v reason=%q", allowed, reason)
	}
}

func TestPrivacyEngineEgressChecks(t *testing.T) {
	engine := NewPrivacyEngine(PrivacyOptions{
		AllowedEgressDomains: []string{"example.com"},
		BlockedEgressDomains: []string{"blocked.com"},
	})

	allowed := engine.CheckEgress("https://api.example.com/v1")
	if !allowed.IsAllowed {
		t.Fatalf("expected allowlisted egress to pass, got %#v", allowed)
	}
	blocked := engine.CheckEgress("https://blocked.com")
	if blocked.IsAllowed || !blocked.RequiresApproval {
		t.Fatalf("expected blocked egress to be denied, got %#v", blocked)
	}
	local := engine.CheckEgress("http://127.0.0.1:8080")
	if !local.IsAllowed {
		t.Fatalf("expected local egress to be allowed, got %#v", local)
	}
}

func TestPolicyHelpersNormalizeDomainsAndPaths(t *testing.T) {
	workspace := t.TempDir()
	paths := normalizePolicyPaths([]string{"logs", "logs"}, workspace)
	if len(paths) != 1 {
		t.Fatalf("expected deduplicated normalized paths, got %#v", paths)
	}
	domains := normalizePolicyDomains([]string{"https://Example.com/api", "example.com", "*.demo.com/"})
	if len(domains) != 2 {
		t.Fatalf("expected deduplicated normalized domains, got %#v", domains)
	}
	if !domainMatches("api.demo.com", "*.demo.com") {
		t.Fatal("expected wildcard domain to match")
	}
	if domainMatches("demo.org", "example.com") {
		t.Fatal("expected unrelated domain to not match")
	}
	if !isLocalEgressHost("192.168.1.2") || isLocalEgressHost("example.com") {
		t.Fatal("unexpected local egress host detection result")
	}
	if normalizePolicyArtifactPath("notes.txt", workspace) != filepath.Join(workspace, "notes.txt") {
		t.Fatal("expected relative artifact path to resolve against working dir")
	}
}

func TestPrivacyDomainStringAndEvaluateDomain(t *testing.T) {
	engine := NewPrivacyEngine(PrivacyOptions{})
	cases := []struct {
		domain  PrivacyDomain
		name    string
		allowed bool
	}{
		{PrivacyDomainNone, "none", true},
		{PrivacyDomainBrowser, "browser", false},
		{PrivacyDomainChat, "chat", false},
		{PrivacyDomainCredentials, "credentials", false},
		{PrivacyDomainKeys, "keys", false},
		{PrivacyDomainDocuments, "documents", false},
		{PrivacyDomainMedia, "media", false},
		{PrivacyDomainSystem, "system", false},
		{PrivacyDomainNetwork, "network", true},
		{PrivacyDomain(999), "unknown", false},
	}

	for _, tc := range cases {
		if got := tc.domain.String(); got != tc.name {
			t.Fatalf("unexpected domain name for %v: %q", tc.domain, got)
		}
		if tc.domain == PrivacyDomain(999) {
			continue
		}
		result := engine.evaluateDomain(tc.domain, "demo")
		if result.DomainName != tc.name {
			t.Fatalf("unexpected domain name in result for %v: %#v", tc.domain, result)
		}
		if result.IsAllowed != tc.allowed {
			t.Fatalf("unexpected allow decision for %v: %#v", tc.domain, result)
		}
	}
}

func TestPrivacyEngineCheckEgressInvalidURL(t *testing.T) {
	engine := NewPrivacyEngine(PrivacyOptions{})
	result := engine.CheckEgress("://bad")
	if result.IsAllowed || !strings.Contains(result.Reason, "invalid URL") {
		t.Fatalf("expected invalid URL to be denied, got %#v", result)
	}
}

func TestPolicyCommandAndPluginBranches(t *testing.T) {
	workspace := t.TempDir()
	policy := NewPolicyEngine(PolicyOptions{
		WorkingDir:        workspace,
		AllowedWritePaths: []string{workspace},
		PermissionLevel:   "read-only",
	})
	if err := policy.CheckCommandCwd(workspace); err == nil {
		t.Fatal("expected read-only command cwd to be denied")
	}

	policy = NewPolicyEngine(PolicyOptions{
		WorkingDir:           workspace,
		AllowedEgressDomains: []string{"example.com"},
	})
	if err := policy.ValidatePluginPermissions("demo-plugin", []string{"tool:exec"}); err != nil {
		t.Fatalf("expected non-net permissions to pass, got %v", err)
	}
	if err := policy.CheckEgressURL("file:///tmp/demo"); err != nil {
		t.Fatalf("expected file scheme egress to be allowed, got %v", err)
	}
}
