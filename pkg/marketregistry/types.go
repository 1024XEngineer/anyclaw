package marketregistry

type ArtifactKind string

const (
	ArtifactKindAgent ArtifactKind = "agent"
	ArtifactKindSkill ArtifactKind = "skill"
	ArtifactKindCLI   ArtifactKind = "cli"
)

type Artifact struct {
	ID              string            `json:"id"`
	Kind            ArtifactKind      `json:"kind"`
	Name            string            `json:"name"`
	Summary         string            `json:"summary"`
	DescriptionMD   string            `json:"description_md,omitempty"`
	Version         string            `json:"version"`
	LatestVersion   string            `json:"latest_version"`
	Source          string            `json:"source"`
	Publisher       string            `json:"publisher"`
	RiskLevel       string            `json:"risk_level"`
	TrustLevel      string            `json:"trust_level"`
	Permissions     []string          `json:"permissions"`
	Compatibility   Compatibility     `json:"compatibility"`
	Dependencies    []Dependency      `json:"dependencies,omitempty"`
	SizeBytes       int64             `json:"size_bytes,omitempty"`
	ChecksumSHA256  string            `json:"checksum_sha256,omitempty"`
	IconURL         string            `json:"icon_url,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	HitSignals      []string          `json:"hit_signals,omitempty"`
	Score           float64           `json:"score,omitempty"`
	UpdatedAt       string            `json:"updated_at,omitempty"`
	ManifestSummary map[string]string `json:"manifest_summary,omitempty"`
}

type Compatibility struct {
	AnyClawMin string   `json:"anyclaw_min,omitempty"`
	OS         []string `json:"os,omitempty"`
	Arch       []string `json:"arch,omitempty"`
}

type Dependency struct {
	ID           string `json:"id"`
	VersionRange string `json:"version_range,omitempty"`
}

type ArtifactVersion struct {
	ArtifactID      string        `json:"artifact_id,omitempty"`
	Version         string        `json:"version"`
	ReleasedAt      string        `json:"released_at,omitempty"`
	ChangelogMD     string        `json:"changelog_md,omitempty"`
	Compatibility   Compatibility `json:"compatibility,omitempty"`
	Permissions     []string      `json:"permissions,omitempty"`
	PermissionsDiff []string      `json:"permissions_diff,omitempty"`
	SizeBytes       int64         `json:"size_bytes,omitempty"`
	ChecksumSHA256  string        `json:"checksum_sha256,omitempty"`
	Signature       string        `json:"signature,omitempty"`
	StorageKey      string        `json:"-"`
	Deprecated      bool          `json:"deprecated,omitempty"`
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
	ArtifactID     string        `json:"artifact_id"`
	Version        string        `json:"version"`
	DownloadURL    string        `json:"download_url"`
	ChecksumSHA256 string        `json:"checksum_sha256"`
	Signature      string        `json:"signature,omitempty"`
	SizeBytes      int64         `json:"size_bytes"`
	ManifestURL    string        `json:"manifest_url,omitempty"`
	Compatibility  Compatibility `json:"compatibility"`
	Dependencies   []Dependency  `json:"dependencies,omitempty"`
	RiskLevel      string        `json:"risk_level"`
	TrustLevel     string        `json:"trust_level"`
	Permissions    []string      `json:"permissions"`
	Kind           ArtifactKind  `json:"kind"`
	Name           string        `json:"name"`
}

type SearchFilter struct {
	Kind       ArtifactKind `json:"kind,omitempty"`
	Source     string       `json:"source,omitempty"`
	Query      string       `json:"q,omitempty"`
	Risk       string       `json:"risk,omitempty"`
	Trust      string       `json:"trust,omitempty"`
	Tag        string       `json:"tag,omitempty"`
	Permission string       `json:"permission,omitempty"`
	Publisher  string       `json:"publisher,omitempty"`
	OS         string       `json:"os,omitempty"`
	Arch       string       `json:"arch,omitempty"`
	Sort       string       `json:"sort,omitempty"`
	Limit      int          `json:"limit,omitempty"`
	Offset     int          `json:"offset,omitempty"`
}

type ListResult struct {
	Items  []Artifact `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type VersionListResult struct {
	Items []ArtifactVersion `json:"items"`
	Total int               `json:"total"`
}

type ResponseMeta struct {
	ProtocolVersion string `json:"protocol_version,omitempty"`
	Count           int    `json:"count,omitempty"`
}

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Detail  string `json:"detail,omitempty"`
	} `json:"error"`
}

type StoreConfig struct {
	DataDir string
	Driver  string
	DSN     string
}

type RegistryAuditEvent struct {
	ID       int64          `json:"id,omitempty"`
	Event    string         `json:"event_type"`
	Artifact string         `json:"artifact_id,omitempty"`
	Version  string         `json:"version,omitempty"`
	Detail   map[string]any `json:"detail,omitempty"`
	Created  string         `json:"created_at,omitempty"`
}

type RegistryAuditList struct {
	Items []RegistryAuditEvent `json:"items"`
	Total int                  `json:"total"`
}

type DownloadStat struct {
	ArtifactID string `json:"artifact_id"`
	Version    string `json:"version"`
	Count      int    `json:"count"`
	LastAt     string `json:"last_at,omitempty"`
}

type DownloadStatsResult struct {
	Items []DownloadStat `json:"items"`
	Total int            `json:"total"`
}

type QuarantineRecord struct {
	ArtifactID string `json:"artifact_id"`
	Reason     string `json:"reason"`
	CreatedAt  string `json:"created_at"`
}

type ArtifactDeletion struct {
	ArtifactID string `json:"artifact_id"`
	DeletedAt  string `json:"deleted_at"`
}

type PublisherToken struct {
	ID          string `json:"id"`
	PublisherID string `json:"publisher_id"`
	Token       string `json:"token,omitempty"`
	CreatedAt   string `json:"created_at"`
	RevokedAt   string `json:"revoked_at,omitempty"`
}

type PublisherTokenRevocation struct {
	ID          string `json:"id"`
	PublisherID string `json:"publisher_id"`
	RevokedAt   string `json:"revoked_at"`
}

type PublisherTokenList struct {
	Items []PublisherToken `json:"items"`
	Total int              `json:"total"`
}

type PublishRequest struct {
	Artifact Artifact          `json:"artifact"`
	Versions []ArtifactVersion `json:"versions,omitempty"`
}
