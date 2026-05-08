package marketplace

import "strings"

type ArtifactKind string

const (
	ArtifactKindAgent  ArtifactKind = "agent"
	ArtifactKindSkill  ArtifactKind = "skill"
	ArtifactKindCLI    ArtifactKind = "cli"
	ArtifactKindPlugin ArtifactKind = "plugin"
)

type SourceKind string

const (
	SourceLocal SourceKind = "local"
	SourceCloud SourceKind = "cloud"
)

type ArtifactStatus string

const (
	StatusAvailable   ArtifactStatus = "available"
	StatusInstalling  ArtifactStatus = "installing"
	StatusInstalled   ArtifactStatus = "installed"
	StatusBound       ArtifactStatus = "bound"
	StatusActive      ArtifactStatus = "active"
	StatusDisabled    ArtifactStatus = "disabled"
	StatusError       ArtifactStatus = "error"
	StatusRolledBack  ArtifactStatus = "rolled_back"
	StatusQuarantined ArtifactStatus = "quarantined"
)

type DecisionMode string

const (
	DecisionAuto  DecisionMode = "auto"
	DecisionAsk   DecisionMode = "ask"
	DecisionBlock DecisionMode = "block"
)

type Compatibility struct {
	AnyClawMin string   `json:"anyclaw_min,omitempty"`
	OS         []string `json:"os,omitempty"`
	Arch       []string `json:"arch,omitempty"`
}

type ArtifactDependency struct {
	ID           string `json:"id"`
	VersionRange string `json:"version_range,omitempty"`
}

type ArtifactVersion struct {
	Version         string        `json:"version"`
	ReleasedAt      string        `json:"released_at,omitempty"`
	ChangelogMD     string        `json:"changelog_md,omitempty"`
	Compatibility   Compatibility `json:"compatibility,omitempty"`
	PermissionsDiff []string      `json:"permissions_diff,omitempty"`
	SizeBytes       int64         `json:"size_bytes,omitempty"`
	Deprecated      bool          `json:"deprecated,omitempty"`
}

type Artifact struct {
	ID            string               `json:"id"`
	Kind          ArtifactKind         `json:"kind"`
	Name          string               `json:"name"`
	DisplayName   string               `json:"display_name,omitempty"`
	Description   string               `json:"description,omitempty"`
	Version       string               `json:"version,omitempty"`
	LatestVersion string               `json:"latest_version,omitempty"`
	Source        SourceKind           `json:"source"`
	SourceID      string               `json:"source_id,omitempty"`
	Status        ArtifactStatus       `json:"status"`
	Installed     bool                 `json:"installed"`
	Bound         bool                 `json:"bound"`
	Active        bool                 `json:"active"`
	Enabled       bool                 `json:"enabled"`
	Owner         string               `json:"owner,omitempty"`
	Category      string               `json:"category,omitempty"`
	Tags          []string             `json:"tags,omitempty"`
	Permissions   []string             `json:"permissions,omitempty"`
	RiskLevel     string               `json:"risk_level,omitempty"`
	TrustLevel    string               `json:"trust_level,omitempty"`
	Verified      bool                 `json:"verified,omitempty"`
	Compatibility Compatibility        `json:"compatibility,omitempty"`
	Dependencies  []ArtifactDependency `json:"dependencies,omitempty"`
	HitSignals    []string             `json:"hit_signals,omitempty"`
	Score         float64              `json:"score,omitempty"`
	InstallHint   string               `json:"install_hint,omitempty"`
	TargetHints   []string             `json:"target_hints,omitempty"`
	Capabilities  []string             `json:"capabilities,omitempty"`
	Metadata      map[string]string    `json:"metadata,omitempty"`
}

type JobState string

const (
	JobPending     JobState = "pending"
	JobRunning     JobState = "running"
	JobRollingBack JobState = "rolling_back"
	JobSucceeded   JobState = "succeeded"
	JobFailed      JobState = "failed"
	JobCanceled    JobState = "canceled"
	JobRolledBack  JobState = "rolled_back"
	JobInterrupted JobState = "interrupted"
)

type InstallRequest struct {
	ArtifactID        string `json:"artifact_id"`
	VersionConstraint string `json:"version_constraint,omitempty"`
	InstalledBy       string `json:"installed_by,omitempty"`
	UserConfirmed     bool   `json:"user_confirmed,omitempty"`
	RiskAcknowledged  bool   `json:"risk_acknowledged,omitempty"`
	IdempotencyKey    string `json:"-"`
}

type UpgradeRequest struct {
	ArtifactID        string `json:"artifact_id"`
	VersionConstraint string `json:"version_constraint,omitempty"`
	InstalledBy       string `json:"installed_by,omitempty"`
	UserConfirmed     bool   `json:"user_confirmed,omitempty"`
	RiskAcknowledged  bool   `json:"risk_acknowledged,omitempty"`
	IdempotencyKey    string `json:"-"`
}

type UninstallRequest struct {
	ArtifactID string `json:"artifact_id"`
	ReceiptID  string `json:"receipt_id,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

type UninstallResult struct {
	ArtifactID       string   `json:"artifact_id"`
	ReceiptID        string   `json:"receipt_id"`
	RemovedBindings  []string `json:"removed_bindings"`
	RemovedPath      string   `json:"removed_path,omitempty"`
	UninstalledAt    string   `json:"uninstalled_at"`
	PreviousVersion  string   `json:"previous_version,omitempty"`
	UndoAvailableSec int      `json:"undo_available_seconds,omitempty"`
}

type PolicyDecision struct {
	Decision                    DecisionMode `json:"decision"`
	Reason                      string       `json:"reason,omitempty"`
	Reasons                     []string     `json:"reasons,omitempty"`
	RequiresUserConfirmation    bool         `json:"requires_user_confirmation,omitempty"`
	RequiresRiskAcknowledgement bool         `json:"requires_risk_acknowledgement,omitempty"`
	RiskLevel                   string       `json:"risk_level,omitempty"`
	TrustLevel                  string       `json:"trust_level,omitempty"`
	Permissions                 []string     `json:"permissions,omitempty"`
	HighRiskPermissions         []string     `json:"high_risk_permissions,omitempty"`
}

type InstallJob struct {
	ID                string            `json:"id"`
	Type              string            `json:"type"`
	State             JobState          `json:"state"`
	ArtifactID        string            `json:"artifact_id"`
	Version           string            `json:"version,omitempty"`
	VersionConstraint string            `json:"version_constraint,omitempty"`
	ProgressStep      string            `json:"progress_step,omitempty"`
	ProgressIndex     int               `json:"progress_index"`
	ProgressTotal     int               `json:"progress_total"`
	Error             string            `json:"error,omitempty"`
	ReceiptID         string            `json:"receipt_id,omitempty"`
	InstalledPath     string            `json:"installed_path,omitempty"`
	ChecksumSHA256    string            `json:"checksum_sha256,omitempty"`
	IdempotencyKey    string            `json:"idempotency_key,omitempty"`
	InstalledBy       string            `json:"installed_by,omitempty"`
	Decision          *PolicyDecision   `json:"decision,omitempty"`
	RolledBack        bool              `json:"rolled_back,omitempty"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
	CompletedAt       string            `json:"completed_at,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type JobListResult struct {
	Items []InstallJob `json:"items"`
	Total int          `json:"total"`
}

type InstallReceipt struct {
	ID             string               `json:"id"`
	JobID          string               `json:"job_id"`
	ArtifactID     string               `json:"artifact_id"`
	Kind           ArtifactKind         `json:"kind"`
	Name           string               `json:"name"`
	Description    string               `json:"description,omitempty"`
	Version        string               `json:"version"`
	Source         SourceKind           `json:"source"`
	SourceID       string               `json:"source_id,omitempty"`
	InstalledPath  string               `json:"installed_path"`
	InstalledBy    string               `json:"installed_by"`
	InstalledAt    string               `json:"installed_at"`
	ChecksumSHA256 string               `json:"checksum_sha256"`
	Permissions    []string             `json:"permissions,omitempty"`
	RiskLevel      string               `json:"risk_level,omitempty"`
	TrustLevel     string               `json:"trust_level,omitempty"`
	Compatibility  Compatibility        `json:"compatibility,omitempty"`
	Dependencies   []ArtifactDependency `json:"dependencies,omitempty"`
	Decision       *PolicyDecision      `json:"decision,omitempty"`
}

type BindingTargetType string

const (
	TargetMainAgent          BindingTargetType = "main_agent"
	TargetPersistentSubagent BindingTargetType = "persistent_subagent"
	TargetWorkspace          BindingTargetType = "workspace"
	TargetRuntimeGlobal      BindingTargetType = "runtime_global"
)

type BindingState string

const (
	BindingEnabled  BindingState = "enabled"
	BindingDisabled BindingState = "disabled"
)

type BindingRequest struct {
	ArtifactID string            `json:"artifact_id"`
	ReceiptID  string            `json:"receipt_id,omitempty"`
	TargetType BindingTargetType `json:"target_type"`
	TargetID   string            `json:"target_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Binding struct {
	ID         string            `json:"id"`
	ArtifactID string            `json:"artifact_id"`
	ReceiptID  string            `json:"receipt_id"`
	Kind       ArtifactKind      `json:"kind"`
	Version    string            `json:"version"`
	TargetType BindingTargetType `json:"target_type"`
	TargetID   string            `json:"target_id"`
	TargetName string            `json:"target_name,omitempty"`
	State      BindingState      `json:"state"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type BindingListResult struct {
	Items []Binding `json:"items"`
	Total int       `json:"total"`
}

type MarketAuditEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	ArtifactID string         `json:"artifact_id,omitempty"`
	JobID      string         `json:"job_id,omitempty"`
	BindingID  string         `json:"binding_id,omitempty"`
	Actor      string         `json:"actor,omitempty"`
	Decision   string         `json:"decision,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

type MarketEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Level      string         `json:"level,omitempty"`
	Message    string         `json:"message,omitempty"`
	ArtifactID string         `json:"artifact_id,omitempty"`
	JobID      string         `json:"job_id,omitempty"`
	BindingID  string         `json:"binding_id,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

type MarketEventListResult struct {
	Items []MarketEvent `json:"items"`
	Total int           `json:"total"`
}

type CapabilityIndexItem struct {
	ArtifactID   string       `json:"artifact_id"`
	Kind         ArtifactKind `json:"kind"`
	Name         string       `json:"name"`
	Source       SourceKind   `json:"source"`
	Status       string       `json:"status"`
	Capabilities []string     `json:"capabilities,omitempty"`
	Permissions  []string     `json:"permissions,omitempty"`
	RiskLevel    string       `json:"risk_level,omitempty"`
	TrustLevel   string       `json:"trust_level,omitempty"`
	Score        float64      `json:"score,omitempty"`
}

type CapabilityRoute struct {
	Need           string                `json:"need"`
	InstalledMatch *CapabilityIndexItem  `json:"installed_match,omitempty"`
	CloudMatches   []CapabilityIndexItem `json:"cloud_matches,omitempty"`
	Action         string                `json:"action"`
	Reason         string                `json:"reason"`
}

type Filter struct {
	Kind       ArtifactKind
	Source     SourceKind
	Query      string
	Status     ArtifactStatus
	Risk       string
	Trust      string
	Tag        string
	Permission string
	Publisher  string
	OS         string
	Arch       string
	Sort       string
	Limit      int
	Offset     int
}

type ListResult struct {
	Items  []Artifact `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

func NormalizeKind(value string) ArtifactKind {
	switch ArtifactKind(strings.ToLower(strings.TrimSpace(value))) {
	case ArtifactKindAgent:
		return ArtifactKindAgent
	case ArtifactKindSkill:
		return ArtifactKindSkill
	case ArtifactKindCLI:
		return ArtifactKindCLI
	case ArtifactKindPlugin:
		return ArtifactKindPlugin
	default:
		return ""
	}
}

func NormalizeSource(value string) SourceKind {
	switch SourceKind(strings.ToLower(strings.TrimSpace(value))) {
	case SourceLocal:
		return SourceLocal
	case SourceCloud:
		return SourceCloud
	default:
		return ""
	}
}

func NormalizeStatus(value string) ArtifactStatus {
	switch ArtifactStatus(strings.ToLower(strings.TrimSpace(value))) {
	case StatusAvailable:
		return StatusAvailable
	case StatusInstalling:
		return StatusInstalling
	case StatusInstalled:
		return StatusInstalled
	case StatusBound:
		return StatusBound
	case StatusActive:
		return StatusActive
	case StatusDisabled:
		return StatusDisabled
	case StatusError:
		return StatusError
	case StatusRolledBack:
		return StatusRolledBack
	case StatusQuarantined:
		return StatusQuarantined
	default:
		return ""
	}
}

func NormalizeJobState(value string) JobState {
	switch JobState(strings.ToLower(strings.TrimSpace(value))) {
	case JobPending:
		return JobPending
	case JobRunning:
		return JobRunning
	case JobRollingBack:
		return JobRollingBack
	case JobSucceeded:
		return JobSucceeded
	case JobFailed:
		return JobFailed
	case JobCanceled:
		return JobCanceled
	case JobRolledBack:
		return JobRolledBack
	case JobInterrupted:
		return JobInterrupted
	default:
		return ""
	}
}

func NormalizeBindingTargetType(value string) BindingTargetType {
	switch BindingTargetType(strings.ToLower(strings.TrimSpace(value))) {
	case TargetMainAgent:
		return TargetMainAgent
	case TargetPersistentSubagent:
		return TargetPersistentSubagent
	case TargetWorkspace:
		return TargetWorkspace
	case TargetRuntimeGlobal:
		return TargetRuntimeGlobal
	default:
		return ""
	}
}
