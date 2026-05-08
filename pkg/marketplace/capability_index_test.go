package marketplace

import "testing"

func TestRouteCapabilityNeedPrefersInstalledCapability(t *testing.T) {
	installed := []Artifact{{
		ID:           "cloud.skill.release-notes",
		Kind:         ArtifactKindSkill,
		Name:         "Release Notes Writer",
		Source:       SourceLocal,
		Status:       StatusInstalled,
		Capabilities: []string{"release notes", "changelog"},
	}}
	cloud := []Artifact{{
		ID:           "cloud.skill.release-notes-v2",
		Kind:         ArtifactKindSkill,
		Name:         "Release Notes Writer v2",
		Source:       SourceCloud,
		Status:       StatusAvailable,
		Capabilities: []string{"release notes"},
	}}
	route := RouteCapabilityNeed("please write release notes", installed, cloud, 5)
	if route.Action != "use_installed" || route.InstalledMatch == nil || route.InstalledMatch.ArtifactID != "cloud.skill.release-notes" {
		t.Fatalf("route = %#v, want installed match", route)
	}
}

func TestRouteCapabilityNeedSuggestsCloudInstall(t *testing.T) {
	route := RouteCapabilityNeed("review this pull request", nil, []Artifact{{
		ID:           "cloud.agent.code-reviewer",
		Kind:         ArtifactKindAgent,
		Name:         "Code Reviewer",
		Source:       SourceCloud,
		Status:       StatusAvailable,
		Capabilities: []string{"code review", "pull request"},
	}}, 5)
	if route.Action != "install_from_market" || len(route.CloudMatches) != 1 {
		t.Fatalf("route = %#v, want cloud match", route)
	}
}
