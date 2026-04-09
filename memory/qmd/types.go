package qmd

import (
	"time"
)

// QMDQueryResult QMD 查询结果
type QMDQueryResult struct {
	Path       string  `json:"path"`
	Line       int     `json:"line"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
	Collection string  `json:"collection"`
}

// QMDStatus QMD 状态信息
type QMDStatus struct {
	Available       bool      `json:"available"`
	Version         string    `json:"version,omitempty"`
	Collections     []string  `json:"collections,omitempty"`
	LastUpdated     time.Time `json:"last_updated,omitempty"`
	LastEmbed       time.Time `json:"last_embed,omitempty"`
	IndexedFiles    int       `json:"indexed_files,omitempty"`
	TotalDocuments  int       `json:"total_documents,omitempty"`
	TotalEmbeddings int       `json:"total_embeddings,omitempty"`
	Error           string    `json:"error,omitempty"`
	FallbackEnabled bool      `json:"fallback_enabled"`
}

// QMDCollection QMD 集合配置
type QMDCollection struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Pattern       string    `json:"pattern"`
	CreatedAt     time.Time `json:"created_at"`
	LastUpdate    time.Time `json:"last_updated"`
	DocumentCount int       `json:"document_count"`
}

// QMDConfig QMD 配置（从 config.MemoryConfig.QMD 映射）
type QMDConfig struct {
	Command        string
	Enabled        bool
	IncludeDefault bool
	Paths          []QMDPathConfig
	Sessions       QMDSessionsConfig
	Update         QMDUpdateConfig
	Limits         QMDLimitsConfig
}

// QMDPathConfig QMD 路径配置
type QMDPathConfig struct {
	Name    string
	Path    string
	Pattern string
}

// QMDSessionsConfig QMD 会话配置
type QMDSessionsConfig struct {
	Enabled       bool
	ExportDir     string
	RetentionDays int
}

// QMDUpdateConfig QMD 更新配置
type QMDUpdateConfig struct {
	Interval       time.Duration
	OnBoot         bool
	EmbedInterval  time.Duration
	CommandTimeout time.Duration
	UpdateTimeout  time.Duration
}

// QMDLimitsConfig QMD 搜索限制配置
type QMDLimitsConfig struct {
	MaxResults      int
	MaxSnippetChars int
	TimeoutMs       int
}

// DefaultQMDConfig 返回默认 QMD 配置
func DefaultQMDConfig() QMDConfig {
	return QMDConfig{
		Command:        "qmd",
		Enabled:        false,
		IncludeDefault: true,
		Paths:          []QMDPathConfig{},
		Sessions: QMDSessionsConfig{
			Enabled:       false,
			ExportDir:     "",
			RetentionDays: 30,
		},
		Update: QMDUpdateConfig{
			Interval:       5 * time.Minute,
			OnBoot:         true,
			EmbedInterval:  60 * time.Minute,
			CommandTimeout: 30 * time.Second,
			UpdateTimeout:  120 * time.Second,
		},
		Limits: QMDLimitsConfig{
			MaxResults:      6,
			MaxSnippetChars: 700,
			TimeoutMs:       4000,
		},
	}
}
