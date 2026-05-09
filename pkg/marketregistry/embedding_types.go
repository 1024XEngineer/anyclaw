package marketregistry

const (
	artifactEmbeddingStatusPending = "pending"
	artifactEmbeddingStatusReady   = "ready"
	artifactEmbeddingStatusFailed  = "failed"

	artifactEmbeddingJobType      = "artifact_embedding"
	artifactEmbeddingJobQueued    = "queued"
	artifactEmbeddingJobRunning   = "running"
	artifactEmbeddingJobSucceeded = "succeeded"
	artifactEmbeddingJobFailed    = "failed"
)

type ArtifactEmbedding struct {
	ArtifactID     string    `json:"artifact_id"`
	Model          string    `json:"model"`
	Vector         []float32 `json:"vector,omitempty"`
	VectorDim      int       `json:"vector_dim"`
	EmbeddingText  string    `json:"embedding_text,omitempty"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	LastJobError   string    `json:"last_job_error,omitempty"`
	LastJobStatus  string    `json:"last_job_status,omitempty"`
	LastAttemptAt  string    `json:"last_attempt_at,omitempty"`
	LastUpdatedAt  string    `json:"last_updated_at,omitempty"`
}

type ArtifactEmbeddingJob struct {
	ID           int64  `json:"id"`
	ArtifactID   string `json:"artifact_id"`
	JobType      string `json:"job_type"`
	Status       string `json:"status"`
	AttemptCount int    `json:"attempt_count"`
	Error        string `json:"error,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type ArtifactEmbeddingJobList struct {
	Items []ArtifactEmbeddingJob `json:"items"`
	Total int                    `json:"total"`
}

type ArtifactEmbeddingList struct {
	Items []ArtifactEmbedding `json:"items"`
	Total int                 `json:"total"`
}

type embeddingAdminStats struct {
	QueuedJobs       int `json:"queued_jobs"`
	FailedJobs       int `json:"failed_jobs"`
	ReadyEmbeddings  int `json:"ready_embeddings"`
	PendingEmbedding int `json:"pending_embeddings"`
}
