package index

import (
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/vec"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusCreating   Status = "creating"
	StatusReady      Status = "ready"
	StatusUpdating   Status = "updating"
	StatusRebuilding Status = "rebuilding"
	StatusError      Status = "error"
	StatusDeleting   Status = "deleting"
	StatusDeleted    Status = "deleted"
)

type Config struct {
	Name       string
	Dimensions int
	Distance   vec.DistanceMetric
	Metadata   []string
	AuxColumns []string
	TableName  string
}

func (c Config) TableNameOrDefault() string {
	if c.TableName != "" {
		return c.TableName
	}
	return "vec_" + c.Name
}

func (c Config) normalized() Config {
	normalized := c
	if normalized.Distance == "" {
		normalized.Distance = vec.DistanceCosine
	}
	normalized.Metadata = cloneStringSlice(c.Metadata)
	normalized.AuxColumns = cloneStringSlice(c.AuxColumns)
	normalized.TableName = c.TableNameOrDefault()
	return normalized
}

type IndexInfo struct {
	Name        string    `json:"name"`
	TableName   string    `json:"table_name"`
	Dimensions  int       `json:"dimensions"`
	Distance    string    `json:"distance"`
	Metadata    []string  `json:"metadata"`
	AuxColumns  []string  `json:"aux_columns"`
	Status      Status    `json:"status"`
	VectorCount int64     `json:"vector_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
}

type Progress struct {
	Total     int
	Processed int
	Failed    int
	Elapsed   time.Duration
	ETA       time.Duration
	CurrentID int64
	Message   string
	Done      bool
}

type ProgressFunc func(p Progress)

type IndexItem struct {
	ID       int64
	Text     string
	Vector   []float32
	Metadata map[string]string
	Aux      map[string]string
}

type IndexResult struct {
	IndexName   string        `json:"index_name"`
	Total       int           `json:"total"`
	Indexed     int           `json:"indexed"`
	Failed      int           `json:"failed"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
}

func cloneIndexInfo(info *IndexInfo) *IndexInfo {
	if info == nil {
		return nil
	}

	cloned := *info
	cloned.Metadata = cloneStringSlice(info.Metadata)
	cloned.AuxColumns = cloneStringSlice(info.AuxColumns)
	return &cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
