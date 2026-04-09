package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"anyclaw/config"
	"anyclaw/memory"
	"anyclaw/memory/qmd"
	"github.com/spf13/cobra"
)

// MemoryCmd 记忆管理命令
var MemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage anyclaw memory",
	Long:  `View status, index, and search memory stores. Supports builtin and QMD backends.`,
}

// memoryStatusCmd 显示记忆状态
var memoryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory index statistics",
	Long:  `Display statistics about the memory store including backend type, collections, and documents.`,
	Run:   runMemoryStatus,
}

// memoryIndexCmd 重新索引记忆文件
var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Reindex memory files",
	Long:  `Rebuild the memory index from configured sources.`,
	Run:   runMemoryIndex,
}

// memorySearchCmd 语义搜索记忆
var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search over memory",
	Long:  `Perform semantic search over stored memories using the configured backend.`,
	Args:  cobra.ExactArgs(1),
	Run:   runMemorySearch,
}

// memoryBackendCmd 查看当前后端
var memoryBackendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Show current memory backend",
	Long:  `Display the current memory backend configuration.`,
	Run:   runMemoryBackend,
}

var (
	memorySearchLimit    int
	memorySearchMinScore float64
	memorySearchJSON     bool
	memoryForceBuiltin   bool
)

func init() {
	MemoryCmd.AddCommand(memoryStatusCmd)
	MemoryCmd.AddCommand(memoryIndexCmd)
	MemoryCmd.AddCommand(memorySearchCmd)
	MemoryCmd.AddCommand(memoryBackendCmd)

	memorySearchCmd.Flags().IntVarP(&memorySearchLimit, "limit", "n", 10, "Maximum number of results")
	memorySearchCmd.Flags().Float64Var(&memorySearchMinScore, "min-score", 0.7, "Minimum similarity score (0-1)")
	memorySearchCmd.Flags().BoolVar(&memorySearchJSON, "json", false, "Output in JSON format")

	memoryIndexCmd.Flags().BoolVar(&memoryForceBuiltin, "builtin", false, "Force using builtin backend")
}

// getWorkspace 获取工作区路径
func getWorkspace() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	cfg, err := config.Load("")
	if err != nil {
		// 使用默认工作区
		return filepath.Join(home, ".anyclaw", "workspace"), nil
	}

	if cfg.Workspace.Path != "" {
		return cfg.Workspace.Path, nil
	}

	return filepath.Join(home, ".anyclaw", "workspace"), nil
}

// getSearchManager 获取搜索管理器
func getSearchManager() (memory.MemorySearchManager, error) {
	workspace, err := getWorkspace()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load("")
	if err != nil {
		// 使用默认配置
		cfg = &config.Config{
			Memory: config.MemoryConfig{
				Backend: "builtin",
				Builtin: config.BuiltinMemoryConfig{
					Enabled: true,
				},
			},
		}
	}

	// 如果强制使用 builtin
	if memoryForceBuiltin {
		cfg.Memory.Backend = "builtin"
	}

	return memory.GetMemorySearchManager(cfg.Memory, workspace)
}

// runMemoryStatus 执行记忆状态命令
func runMemoryStatus(cmd *cobra.Command, args []string) {
	mgr, err := getSearchManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create search manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	status := mgr.GetStatus()

	// Display status
	fmt.Println("Memory Status")
	fmt.Println("=============")

	// Backend
	if backend, ok := status["backend"].(string); ok {
		fmt.Printf("\nBackend: %s\n", backend)
	}

	// QMD specific
	if available, ok := status["available"].(bool); ok {
		if available {
			fmt.Println("  Status: Available")
		} else {
			fmt.Println("  Status: Unavailable")
		}
	}

	if collections, ok := status["collections"].([]string); ok {
		fmt.Printf("  Collections: %v\n", collections)
	}

	if indexedFiles, ok := status["indexed_files"].(int); ok {
		fmt.Printf("  Indexed Files: %d\n", indexedFiles)
	}

	if totalDocs, ok := status["total_documents"].(int); ok {
		fmt.Printf("  Total Documents: %d\n", totalDocs)
	}

	if lastUpdated, ok := status["last_updated"].(time.Time); ok && !lastUpdated.IsZero() {
		fmt.Printf("  Last Updated: %s\n", lastUpdated.Format(time.RFC3339))
	}

	if lastEmbed, ok := status["last_embed"].(time.Time); ok && !lastEmbed.IsZero() {
		fmt.Printf("  Last Embed: %s\n", lastEmbed.Format(time.RFC3339))
	}

	// Builtin specific
	if dbPath, ok := status["database_path"].(string); ok {
		fmt.Printf("\nDatabase: %s\n", dbPath)
	}

	if totalCount, ok := status["total_count"].(int); ok {
		fmt.Printf("Total Entries: %d\n", totalCount)
	}

	if sourceCounts, ok := status["source_counts"].(map[memory.MemorySource]int); ok {
		fmt.Println("\nBy Source:")
		for source, count := range sourceCounts {
			fmt.Printf("  %s: %d\n", source, count)
		}
	}

	if typeCounts, ok := status["type_counts"].(map[memory.MemoryType]int); ok {
		fmt.Println("\nBy Type:")
		for memType, count := range typeCounts {
			fmt.Printf("  %s: %d\n", memType, count)
		}
	}

	// Fallback status
	if fallbackEnabled, ok := status["fallback_enabled"].(bool); ok && fallbackEnabled {
		fmt.Println("\nNote: Running in fallback mode (builtin)")
		if fallbackStatus, ok := status["fallback_status"].(map[string]interface{}); ok {
			fmt.Println("Fallback Status:")
			for k, v := range fallbackStatus {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
	}

	// Error message
	if errMsg, ok := status["error"].(string); ok && errMsg != "" {
		fmt.Printf("\nError: %s\n", errMsg)
	}
}

// runMemoryBackend 显示当前后端
func runMemoryBackend(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("Backend: builtin (default)\n")
		return
	}

	backend := cfg.Memory.Backend
	if backend == "" {
		backend = "builtin"
	}

	fmt.Printf("Backend: %s\n", backend)

	if backend == "qmd" {
		fmt.Printf("  QMD Command: %s\n", cfg.Memory.QMD.Command)
		fmt.Printf("  Enabled: %v\n", cfg.Memory.QMD.Enabled)
		if len(cfg.Memory.QMD.Paths) > 0 {
			fmt.Println("  Paths:")
			for _, p := range cfg.Memory.QMD.Paths {
				fmt.Printf("    - %s: %s (%s)\n", p.Name, p.Path, p.Pattern)
			}
		}
		if cfg.Memory.QMD.Sessions.Enabled {
			fmt.Printf("  Sessions Export: %s\n", cfg.Memory.QMD.Sessions.ExportDir)
		}
	}
}

// runMemoryIndex 执行记忆索引命令
func runMemoryIndex(cmd *cobra.Command, args []string) {
	workspace, err := getWorkspace()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get workspace: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load("")
	if err != nil {
		cfg = &config.Config{}
	}

	// 如果强制使用 builtin 或配置为 builtin
	if memoryForceBuiltin || cfg.Memory.Backend == "builtin" || cfg.Memory.Backend == "" {
		runBuiltinIndex(workspace, cfg)
		return
	}

	// QMD 模式
	if cfg.Memory.Backend == "qmd" {
		runQMDIndex(workspace, cfg)
		return
	}

	fmt.Fprintf(os.Stderr, "Unknown backend: %s\n", cfg.Memory.Backend)
	os.Exit(1)
}

// runBuiltinIndex 执行 builtin 索引
func runBuiltinIndex(workspace string, cfg *config.Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	memoryDir := filepath.Join(workspace, "memory")
	dbPath := filepath.Join(home, ".anyclaw", "memory", "store.db")

	// Load config for API key - use models.providers format
	var apiKey string
	for _, provider := range cfg.Models.Providers {
		if provider.APIKey != "" {
			apiKey = provider.APIKey
			break
		}
	}

	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: No embedding provider API key found in config.\n")
		fmt.Fprintf(os.Stderr, "Please configure a provider API key in models.providers in ~/.anyclaw/config.json\n")
		os.Exit(1)
	}

	// Create embedding provider
	providerCfg := memory.DefaultOpenAIConfig(apiKey)
	provider, err := memory.NewOpenAIProvider(providerCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create embedding provider: %v\n", err)
		os.Exit(1)
	}

	// Create store
	storeConfig := memory.DefaultStoreConfig(dbPath, provider)
	store, err := memory.NewSQLiteStore(storeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open memory store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create memory manager
	managerConfig := memory.DefaultManagerConfig(store, provider)
	manager, err := memory.NewMemoryManager(managerConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create memory manager: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	fmt.Println("Indexing memory files (builtin backend)...")
	fmt.Printf("Workspace: %s\n", workspace)
	fmt.Printf("Database: %s\n\n", dbPath)

	ctx := context.Background()

	// Index MEMORY.md
	longTermPath := filepath.Join(memoryDir, "MEMORY.md")
	if _, err := os.Stat(longTermPath); err == nil {
		fmt.Printf("Indexing %s...\n", longTermPath)
		if err := indexFile(ctx, manager, longTermPath, memory.MemorySourceLongTerm, memory.MemoryTypeFact); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to index %s: %v\n", longTermPath, err)
		} else {
			fmt.Println("  OK")
		}
	} else {
		fmt.Printf("No long-term memory file found (%s)\n", longTermPath)
	}

	// Index daily notes
	dailyFiles, err := filepath.Glob(filepath.Join(memoryDir, "????-??-??.md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to find daily notes: %v\n", err)
	} else {
		fmt.Printf("\nIndexing daily notes (%d files)...\n", len(dailyFiles))
		for _, dailyFile := range dailyFiles {
			fmt.Printf("  %s...", filepath.Base(dailyFile))
			if err := indexFile(ctx, manager, dailyFile, memory.MemorySourceDaily, memory.MemoryTypeContext); err != nil {
				fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
			} else {
				fmt.Println(" OK")
			}
		}
	}

	fmt.Println("\nIndexing complete!")
}

// runQMDIndex 执行 QMD 索引
func runQMDIndex(workspace string, cfg *config.Config) {
	fmt.Println("Indexing memory files (QMD backend)...")

	// Create QMD config
	qmdCfg := cfg.Memory.QMD
	qmdMgrConfig := qmd.QMDConfig{
		Command:        qmdCfg.Command,
		Enabled:        qmdCfg.Enabled,
		IncludeDefault: qmdCfg.IncludeDefault,
		Paths:          make([]qmd.QMDPathConfig, len(qmdCfg.Paths)),
		Sessions: qmd.QMDSessionsConfig{
			Enabled:       qmdCfg.Sessions.Enabled,
			ExportDir:     qmdCfg.Sessions.ExportDir,
			RetentionDays: qmdCfg.Sessions.RetentionDays,
		},
		Update: qmd.QMDUpdateConfig{
			Interval:       qmdCfg.Update.Interval,
			OnBoot:         qmdCfg.Update.OnBoot,
			EmbedInterval:  qmdCfg.Update.EmbedInterval,
			CommandTimeout: qmdCfg.Update.CommandTimeout,
			UpdateTimeout:  qmdCfg.Update.UpdateTimeout,
		},
		Limits: qmd.QMDLimitsConfig{
			MaxResults:      qmdCfg.Limits.MaxResults,
			MaxSnippetChars: qmdCfg.Limits.MaxSnippetChars,
			TimeoutMs:       qmdCfg.Limits.TimeoutMs,
		},
	}

	for i, p := range qmdCfg.Paths {
		qmdMgrConfig.Paths[i] = qmd.QMDPathConfig{
			Name:    p.Name,
			Path:    p.Path,
			Pattern: p.Pattern,
		}
	}

	qmdMgr := qmd.NewQMDManager(qmdMgrConfig, workspace, "")

	// Initialize
	ctx, cancel := context.WithTimeout(context.Background(), qmdCfg.Update.UpdateTimeout)
	defer cancel()

	if err := qmdMgr.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize QMD manager: %v\n", err)
		os.Exit(1)
	}
	defer qmdMgr.Close()

	// Update
	fmt.Println("Updating QMD collections...")
	if err := qmdMgr.Update(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update collections: %v\n", err)
	}

	// Embed
	fmt.Println("Generating embeddings...")
	if err := qmdMgr.Embed(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to generate embeddings: %v\n", err)
	}

	// Show status
	status := qmdMgr.GetStatus()
	fmt.Println("\nIndexing complete!")
	fmt.Printf("Collections: %v\n", status.Collections)
	fmt.Printf("Indexed Files: %d\n", status.IndexedFiles)
	fmt.Printf("Total Documents: %d\n", status.TotalDocuments)
}

// runMemorySearch 执行记忆搜索命令
func runMemorySearch(cmd *cobra.Command, args []string) {
	query := args[0]

	mgr, err := getSearchManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create search manager: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	// Perform search
	ctx := context.Background()
	opts := memory.DefaultSearchOptions()
	opts.Limit = memorySearchLimit
	opts.MinScore = memorySearchMinScore

	results, err := mgr.Search(ctx, query, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
		os.Exit(1)
	}

	if memorySearchJSON {
		outputSearchResultsJSON(query, results)
		return
	}

	outputSearchResults(query, results)
}

// outputSearchResultsJSON 输出搜索结果为 JSON
func outputSearchResultsJSON(query string, results []*memory.SearchResult) {
	data := struct {
		Query   string                 `json:"query"`
		Count   int                    `json:"count"`
		Results []*memory.SearchResult `json:"results"`
	}{
		Query:   query,
		Count:   len(results),
		Results: results,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

// outputSearchResults 输出搜索结果
func outputSearchResults(query string, results []*memory.SearchResult) {
	if len(results) == 0 {
		fmt.Printf("No results found for: %s\n", query)
		return
	}

	fmt.Printf("Search Results for: %s\n", query)
	fmt.Printf("Found %d result(s)\n\n", len(results))

	for i, result := range results {
		fmt.Printf("[%d] Score: %.2f\n", i+1, result.Score)
		if result.Source != "" {
			fmt.Printf("    Source: %s\n", result.Source)
		}
		if result.Type != "" {
			fmt.Printf("    Type: %s\n", result.Type)
		}

		if result.Metadata.FilePath != "" {
			fmt.Printf("    File: %s", result.Metadata.FilePath)
			if result.Metadata.LineNumber > 0 {
				fmt.Printf(":%d", result.Metadata.LineNumber)
			}
			fmt.Println()
		}

		if !result.CreatedAt.IsZero() {
			fmt.Printf("    Created: %s\n", result.CreatedAt.Format(time.RFC3339))
		}

		// Truncate text for display
		text := result.Text
		maxLen := 200
		if len(text) > maxLen {
			text = text[:maxLen] + "..."
		}
		fmt.Printf("    Text: %s\n\n", text)
	}
}

// Helper functions for builtin indexing

// indexFile 索引单个文件
func indexFile(ctx context.Context, manager *memory.MemoryManager, filePath string, source memory.MemorySource, memType memory.MemoryType) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	text := string(content)
	if text == "" {
		return nil
	}

	// Split into chunks (paragraphs)
	chunks := splitIntoChunks(text, 500)

	items := make([]memory.MemoryItem, 0, len(chunks))
	for i, chunk := range chunks {
		items = append(items, memory.MemoryItem{
			Text:   chunk,
			Source: source,
			Type:   memType,
			Metadata: memory.MemoryMetadata{
				FilePath: filePath,
				Tags:     []string{"indexed"},
			},
		})

		// Add line number hint
		if i > 0 {
			items[i-1].Metadata.LineNumber = i * 10
		}
	}

	if len(items) > 0 {
		if err := manager.AddMemoryBatch(ctx, items); err != nil {
			return fmt.Errorf("failed to add memories: %w", err)
		}
	}

	return nil
}

// splitIntoChunks 将文本分割成块
func splitIntoChunks(text string, maxChunkSize int) []string {
	// Simple paragraph-based chunking
	paragraphs := splitParagraphs(text)
	chunks := make([]string, 0)
	currentChunk := ""

	for _, para := range paragraphs {
		if len(currentChunk)+len(para) > maxChunkSize && currentChunk != "" {
			chunks = append(chunks, currentChunk)
			currentChunk = para
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += para
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// splitParagraphs 分割段落
func splitParagraphs(text string) []string {
	// Split by double newline
	paragraphs := make([]string, 0)
	current := ""

	lines := splitLines(text)
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" {
			if current != "" {
				paragraphs = append(paragraphs, current)
				current = ""
			}
		} else {
			if current != "" {
				current += " "
			}
			current += line
		}
	}

	if current != "" {
		paragraphs = append(paragraphs, current)
	}

	return paragraphs
}

// Helper functions to avoid importing strings package
func splitLines(s string) []string {
	lines := make([]string, 0)
	current := ""

	for _, ch := range s {
		if ch == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		lines = append(lines, current)
	}

	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
