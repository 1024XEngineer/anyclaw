# Vector Memory System

This package implements a vector-based memory system for anyclaw with semantic search capabilities.

## Architecture

### Components

1. **Types** (`types.go`) - Core data structures and interfaces
2. **Embeddings** (`embeddings.go`) - Embedding provider implementations
3. **Store** (`store.go`) - SQLite-based persistent storage
4. **Search** (`search.go`) - High-level memory management API
5. **Vector** (`vector.go`) - Vector math utilities

## Features

- **Vector Embeddings**: Support for OpenAI, Gemini, and extensible providers
- **Semantic Search**: Cosine similarity search with configurable thresholds
- **Hybrid Search**: Combine vector similarity with full-text search (FTS5)
- **Memory Types**: Long-term, session, and daily notes
- **Caching**: In-memory LRU cache for frequently accessed memories
- **Batch Operations**: Efficient bulk embedding and storage

## Usage

### Basic Setup

```go
import (
    "anyclaw/memory"
)

// Create embedding provider
provider, err := memory.NewOpenAIProvider(memory.DefaultOpenAIConfig(apiKey))
if err != nil {
    log.Fatal(err)
}

// Create store
store, err := memory.NewSQLiteStore(memory.DefaultStoreConfig(
    "/path/to/memory.db",
    provider,
))
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Create manager
manager, err := memory.NewMemoryManager(memory.DefaultManagerConfig(store, provider))
if err != nil {
    log.Fatal(err)
}
defer manager.Close()
```

### Adding Memories

```go
// Add a single memory
ve, err := manager.AddMemory(ctx, "User prefers dark mode",
    memory.MemorySourceLongTerm,
    memory.MemoryTypePreference,
    memory.MemoryMetadata{
        Tags: []string{"preference", "ui"},
        Importance: 0.8,
    })
if err != nil {
    log.Fatal(err)
}

// Add multiple memories
items := []memory.MemoryItem{
    {
        Text: "User works at TechCorp",
        Source: memory.MemorySourceLongTerm,
        Type: memory.MemoryTypeFact,
    },
    {
        Text: "Meeting scheduled for tomorrow",
        Source: memory.MemorySourceDaily,
        Type: memory.MemoryTypeContext,
    },
}
err = manager.AddMemoryBatch(ctx, items)
```

### Searching

```go
// Semantic search
results, err := manager.Search(ctx, "user preferences",
    memory.SearchOptions{
        Limit:    10,
        MinScore: 0.7,
        Hybrid:   true,
    })
if err != nil {
    log.Fatal(err)
}

for _, result := range results {
    fmt.Printf("Score: %.2f, Text: %s\n", result.Score, result.Text)
}

// Search by tag
preferenceMemories, err := manager.SearchByTag(ctx, "preference")

// Search by source
longTerm, err := manager.SearchBySource(ctx, memory.MemorySourceLongTerm)
```

## Configuration

### Store Options

```go
config := memory.StoreConfig{
    DBPath:              "/path/to/memory.db",
    Provider:            provider,
    EnableVectorSearch:  true,   // Requires sqlite-vec
    EnableFTS:           true,   // Requires FTS5
    VectorExtensionPath: "",     // Auto-detect if empty
}
```

### Search Options

```go
opts := memory.SearchOptions{
    Limit:        10,              // Max results
    MinScore:     0.7,             // Minimum similarity (0-1)
    Sources:      []MemorySource{MemorySourceLongTerm},
    Types:        []MemoryType{MemoryTypeFact},
    Hybrid:       true,            // Enable hybrid search
    VectorWeight: 0.7,             // Weight for vector similarity
    TextWeight:   0.3,             // Weight for keyword match
}
```

## Dependencies

- `github.com/glebarez/sqlite` - Pure Go SQLite driver
- `github.com/google/uuid` - UUID generation

## Optional Extensions

- **sqlite-vec**: Vector similarity search extension
  - Download: https://github.com/asg017/sqlite-vec
  - Place in library path or set `SQLITE_VEC_EXTENSION` env var

- **FTS5**: Full-text search (built into SQLite)

## Integration with Agent Context

To integrate with the agent context builder:

```go
import "anyclaw/memory"

// In agent/context.go
type ContextBuilder struct {
    // ... existing fields
    memoryManager *memory.MemoryManager
}

func (b *ContextBuilder) buildSystemPromptWithSkills(...) string {
    // ... existing code

    // Add relevant memories
    if b.memoryManager != nil {
        results, err := b.memoryManager.Search(ctx,
            "user preferences and context",
            memory.SearchOptions{
                Limit:    5,
                MinScore: 0.7,
            })
        if err == nil && len(results) > 0 {
            parts = append(parts, "## Relevant Memories\n\n")
            for _, r := range results {
                parts = append(parts, fmt.Sprintf("- %s\n", r.Text))
            }
        }
    }

    return fmt.Sprintf("%s\n\n", joinNonEmpty(parts, "\n\n---\n\n"))
}
```

## Performance Considerations

- **Batch Size**: OpenAI supports up to 2048 texts per batch
- **Caching**: Enable for frequently accessed memories
- **Vector Extension**: Optional but recommended for large datasets
- **FTS5**: Faster than substring matching for text search

## Future Enhancements

1. Additional embedding providers (Voyage, local models)
2. Automatic memory file indexing
3. Session transcript integration
4. Memory importance scoring
5. Automatic memory pruning
6. Hierarchical memory organization
