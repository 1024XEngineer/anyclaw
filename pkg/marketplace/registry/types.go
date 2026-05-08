package registry

import "github.com/1024XEngineer/anyclaw/pkg/marketplace"

type remoteArtifact struct {
	ID              string                   `json:"id"`
	Kind            marketplace.ArtifactKind `json:"kind"`
	Name            string                   `json:"name"`
	Summary         string                   `json:"summary"`
	DescriptionMD   string                   `json:"description_md,omitempty"`
	Version         string                   `json:"version"`
	LatestVersion   string                   `json:"latest_version"`
	Source          string                   `json:"source"`
	Publisher       string                   `json:"publisher"`
	RiskLevel       string                   `json:"risk_level"`
	TrustLevel      string                   `json:"trust_level"`
	Permissions     []string                 `json:"permissions"`
	Compatibility   remoteCompatibility      `json:"compatibility"`
	Dependencies    []remoteDependency       `json:"dependencies,omitempty"`
	SizeBytes       int64                    `json:"size_bytes,omitempty"`
	ChecksumSHA256  string                   `json:"checksum_sha256,omitempty"`
	IconURL         string                   `json:"icon_url,omitempty"`
	Tags            []string                 `json:"tags,omitempty"`
	HitSignals      []string                 `json:"hit_signals,omitempty"`
	Score           float64                  `json:"score,omitempty"`
	UpdatedAt       string                   `json:"updated_at,omitempty"`
	ManifestSummary map[string]string        `json:"manifest_summary,omitempty"`
}

type remoteCompatibility struct {
	AnyClawMin string   `json:"anyclaw_min,omitempty"`
	OS         []string `json:"os,omitempty"`
	Arch       []string `json:"arch,omitempty"`
}

type remoteDependency struct {
	ID           string `json:"id"`
	VersionRange string `json:"version_range,omitempty"`
}

type remoteVersion struct {
	ArtifactID      string              `json:"artifact_id,omitempty"`
	Version         string              `json:"version"`
	ReleasedAt      string              `json:"released_at,omitempty"`
	ChangelogMD     string              `json:"changelog_md,omitempty"`
	Compatibility   remoteCompatibility `json:"compatibility,omitempty"`
	Permissions     []string            `json:"permissions,omitempty"`
	PermissionsDiff []string            `json:"permissions_diff,omitempty"`
	SizeBytes       int64               `json:"size_bytes,omitempty"`
	ChecksumSHA256  string              `json:"checksum_sha256,omitempty"`
	Deprecated      bool                `json:"deprecated,omitempty"`
}

type ResolveRequest struct {
	VersionConstraint string `json:"version_constraint,omitempty"`
	ClientEnv         struct {
		AnyClawVersion string `json:"anyclaw_version,omitempty"`
		OS             string `json:"os,omitempty"`
		Arch           string `json:"arch,omitempty"`
	} `json:"client_env,omitempty"`
}

type ResolvedArtifact struct {
	ArtifactID     string                           `json:"artifact_id"`
	Version        string                           `json:"version"`
	DownloadURL    string                           `json:"download_url"`
	ChecksumSHA256 string                           `json:"checksum_sha256"`
	Signature      string                           `json:"signature,omitempty"`
	SizeBytes      int64                            `json:"size_bytes"`
	ManifestURL    string                           `json:"manifest_url,omitempty"`
	Compatibility  marketplace.Compatibility        `json:"compatibility"`
	Dependencies   []marketplace.ArtifactDependency `json:"dependencies,omitempty"`
	RiskLevel      string                           `json:"risk_level"`
	TrustLevel     string                           `json:"trust_level"`
	Permissions    []string                         `json:"permissions"`
	Kind           marketplace.ArtifactKind         `json:"kind"`
	Name           string                           `json:"name"`
}

type listEnvelope struct {
	Data struct {
		Items  []remoteArtifact `json:"items"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	} `json:"data"`
}

type artifactEnvelope struct {
	Data remoteArtifact `json:"data"`
}

type versionsEnvelope struct {
	Data struct {
		Items []remoteVersion `json:"items"`
		Total int             `json:"total"`
	} `json:"data"`
}

type resolveEnvelope struct {
	Data ResolvedArtifact `json:"data"`
}
