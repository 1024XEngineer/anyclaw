package marketregistry

import (
	"context"
	"time"
)

func SeedIfEmpty(ctx context.Context, store *Store, storage *LocalStorage) error {
	count, err := store.CountArtifacts(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return SeedFixtures(ctx, store, storage)
}

func SeedFixtures(ctx context.Context, store *Store, storage *LocalStorage) error {
	now := time.Now().UTC().Format(time.RFC3339)
	fixtures := []Artifact{
		{
			ID:              "cloud.agent.code-reviewer",
			Kind:            ArtifactKindAgent,
			Name:            "Cloud Code Reviewer",
			Summary:         "Reviews local changes and highlights concrete risks before merge.",
			DescriptionMD:   "A marketplace fixture agent for review-oriented workflows.",
			Version:         "1.0.0",
			LatestVersion:   "1.0.0",
			Source:          defaultRegistrySourceID,
			Publisher:       "AnyClaw Labs",
			RiskLevel:       "medium",
			TrustLevel:      "verified",
			Permissions:     []string{"fs.read", "git.read"},
			Compatibility:   Compatibility{AnyClawMin: "0.1.0", OS: []string{"windows", "linux", "darwin"}, Arch: []string{"amd64", "arm64"}},
			Tags:            []string{"agent", "review", "quality"},
			HitSignals:      []string{"code review", "风险检查", "pull request"},
			Score:           0.96,
			UpdatedAt:       now,
			ManifestSummary: map[string]string{"entry": "agent/profile.json"},
		},
		{
			ID:              "cloud.skill.release-notes",
			Kind:            ArtifactKindSkill,
			Name:            "Release Notes Writer",
			Summary:         "Turns git history and issue notes into compact release notes.",
			DescriptionMD:   "A marketplace fixture skill for writing release notes from project context.",
			Version:         "1.0.0",
			LatestVersion:   "1.0.0",
			Source:          defaultRegistrySourceID,
			Publisher:       "AnyClaw Labs",
			RiskLevel:       "low",
			TrustLevel:      "verified",
			Permissions:     []string{"fs.read", "git.read"},
			Compatibility:   Compatibility{AnyClawMin: "0.1.0", OS: []string{"windows", "linux", "darwin"}, Arch: []string{"amd64", "arm64"}},
			Tags:            []string{"skill", "release", "writing"},
			HitSignals:      []string{"release notes", "changelog", "发布说明"},
			Score:           0.94,
			UpdatedAt:       now,
			ManifestSummary: map[string]string{"entry": "skill/SKILL.md"},
		},
		{
			ID:              "cloud.cli.repo-health",
			Kind:            ArtifactKindCLI,
			Name:            "Repo Health CLI",
			Summary:         "Runs a lightweight repository health check command.",
			DescriptionMD:   "A marketplace fixture CLI package for command binding tests.",
			Version:         "1.0.0",
			LatestVersion:   "1.0.0",
			Source:          defaultRegistrySourceID,
			Publisher:       "AnyClaw Labs",
			RiskLevel:       "medium",
			TrustLevel:      "verified",
			Permissions:     []string{"process.exec", "fs.read"},
			Compatibility:   Compatibility{AnyClawMin: "0.1.0", OS: []string{"windows", "linux", "darwin"}, Arch: []string{"amd64", "arm64"}},
			Tags:            []string{"cli", "health", "repository"},
			HitSignals:      []string{"repo health", "诊断", "cli"},
			Score:           0.91,
			UpdatedAt:       now,
			ManifestSummary: map[string]string{"command": "anyclaw-repo-health"},
		},
	}

	for _, artifact := range fixtures {
		version := ArtifactVersion{
			ArtifactID:      artifact.ID,
			Version:         artifact.LatestVersion,
			ReleasedAt:      now,
			ChangelogMD:     "Initial registry fixture.",
			Compatibility:   artifact.Compatibility,
			Permissions:     artifact.Permissions,
			PermissionsDiff: artifact.Permissions,
		}
		info, err := storage.EnsurePackage(artifact, version)
		if err != nil {
			return err
		}
		version.SizeBytes = info.SizeBytes
		version.ChecksumSHA256 = info.ChecksumSHA256
		version.StorageKey = info.StorageKey
		artifact.SizeBytes = info.SizeBytes
		artifact.ChecksumSHA256 = info.ChecksumSHA256
		if err := store.UpsertArtifact(ctx, artifact, []ArtifactVersion{version}); err != nil {
			return err
		}
	}
	return nil
}
