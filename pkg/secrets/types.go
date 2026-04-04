package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

type SecretScope string

const (
	ScopeGlobal   SecretScope = "global"
	ScopeApp      SecretScope = "app"
	ScopeBinding  SecretScope = "binding"
	ScopeWorkflow SecretScope = "workflow"
	ScopeAgent    SecretScope = "agent"
)

type SecretSource string

const (
	SourceManual  SecretSource = "manual"
	SourceEnv     SecretSource = "env"
	SourceFile    SecretSource = "file"
	SourceVault   SecretSource = "vault"
	SourceInstall SecretSource = "install"
)

type SecretEntry struct {
	ID          string            `json:"id"`
	Key         string            `json:"key"`
	Value       string            `json:"value"`
	Scope       SecretScope       `json:"scope"`
	ScopeRef    string            `json:"scope_ref,omitempty"`
	Source      SecretSource      `json:"source"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
}

type Snapshot struct {
	ID        string                  `json:"id"`
	Version   uint64                  `json:"version"`
	Secrets   map[string]*SecretEntry `json:"secrets"`
	CreatedAt time.Time               `json:"created_at"`
	Source    string                  `json:"source,omitempty"`
	Checksum  string                  `json:"checksum"`
}

type ActivationLockState string

const (
	LockUnlocked  ActivationLockState = "unlocked"
	LockPending   ActivationLockState = "pending"
	LockActivated ActivationLockState = "activated"
	LockRevoked   ActivationLockState = "revoked"
	LockExpired   ActivationLockState = "expired"
)

type ActivationLock struct {
	ID            string              `json:"id"`
	SnapshotID    string              `json:"snapshot_id"`
	SnapshotVer   uint64              `json:"snapshot_version"`
	State         ActivationLockState `json:"state"`
	RequestedBy   string              `json:"requested_by"`
	RequestedAt   time.Time           `json:"requested_at"`
	ActivatedBy   string              `json:"activated_by,omitempty"`
	ActivatedAt   *time.Time          `json:"activated_at,omitempty"`
	RevokedBy     string              `json:"revoked_by,omitempty"`
	RevokedAt     *time.Time          `json:"revoked_at,omitempty"`
	ExpiresAt     *time.Time          `json:"expires_at,omitempty"`
	RequiresCount int                 `json:"requires_count"`
	Approvals     []LockApproval      `json:"approvals"`
	Reason        string              `json:"reason,omitempty"`
}

type LockApproval struct {
	Approver   string    `json:"approver"`
	ApprovedAt time.Time `json:"approved_at"`
	Comment    string    `json:"comment,omitempty"`
}

type Operation string

const (
	OpCreate   Operation = "create"
	OpUpdate   Operation = "update"
	OpDelete   Operation = "delete"
	OpSnapshot Operation = "snapshot"
	OpActivate Operation = "activate"
	OpRevoke   Operation = "revoke"
	OpAccess   Operation = "access"
	OpRotate   Operation = "rotate"
)

type AuditEntry struct {
	ID         string                 `json:"id"`
	Operation  Operation              `json:"operation"`
	SecretKey  string                 `json:"secret_key,omitempty"`
	SnapshotID string                 `json:"snapshot_id,omitempty"`
	LockID     string                 `json:"lock_id,omitempty"`
	Actor      string                 `json:"actor"`
	Timestamp  time.Time              `json:"timestamp"`
	Details    map[string]interface{} `json:"details,omitempty"`
	IP         string                 `json:"ip,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	Success    bool                   `json:"success"`
	Error      string                 `json:"error,omitempty"`
}

type StoreConfig struct {
	Path            string        `json:"path"`
	EncryptionKey   string        `json:"encryption_key,omitempty"`
	AutoSnapshot    bool          `json:"auto_snapshot"`
	MaxSnapshots    int           `json:"max_snapshots"`
	LockTimeout     time.Duration `json:"lock_timeout"`
	RequireApproval bool          `json:"require_approval"`
	ApprovalCount   int           `json:"approval_count"`
}

func DefaultStoreConfig() *StoreConfig {
	return &StoreConfig{
		AutoSnapshot:    true,
		MaxSnapshots:    50,
		LockTimeout:     24 * time.Hour,
		RequireApproval: false,
		ApprovalCount:   1,
	}
}

func EncryptValue(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptValue(encoded string, key []byte) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
