# Enhanced Session Management

This package provides advanced session management features including tree-structured conversations, caching, and intelligent pruning.

## Features

### 1. Tree-Structured Sessions (`tree.go`)

Support for conversation branching and versioning:

```go
// Create a new session tree
tree, err := session.NewSessionTree(rootSession)

// Create a branch
branchID, err := tree.CreateBranch(parentID, newSession, "experiment-1", "user")

// Switch between branches
session, err := tree.SwitchBranch(fromID, toID)

// Merge a branch back
err := tree.MergeBranch(branchID)

// Get path from root to node
path, err := tree.GetPath(nodeID)
```

**Key Features:**
- Configurable maximum branch depth (default: 10)
- Branch metadata (name, description, creator)
- Merge tracking
- Circular reference detection
- Path reconstruction
- Session comparison

### 2. In-Memory Cache (`cache.go`)

LRU cache with TTL for frequently accessed sessions:

```go
// Create cache with custom configuration
cache := session.NewCache(session.CacheConfig{
    MaxSize:      100,
    DefaultTTL:   1 * time.Hour,
    CleanupIntvl: 5 * time.Minute,
})
defer cache.Close()

// Get session from cache
sess, ok := cache.Get(key)

// Set session with custom TTL
cache.SetWithTTL(key, session, 30*time.Minute)

// Prune by size or access count
evicted := cache.PruneBySize(10 * 1024 * 1024) // 10MB
evicted = cache.PruneByAccessCount(2)
```

**Key Features:**
- LRU eviction policy
- Per-entry TTL support
- Size-based pruning
- Access count tracking
- Hit rate statistics
- Automatic cleanup routine

### 3. Intelligent Pruning (`prune.go`)

Multiple pruning strategies for session and message management:

```go
// Create pruner with configuration
pruner := session.NewPruner(manager, session.PruneConfig{
    Strategy:           session.PruneStrategyLRU,
    MaxTotalSessions:   1000,
    MaxTotalMessages:   10000,
    DefaultMessageTTL:  24 * time.Hour,
    DMPreserveCount:    100,
    GroupPreserveCount: 50,
})

// Execute pruning
err := pruner.PruneSessions()

// Prune messages within a session
err := pruner.PruneMessages(sessionKey, 100)

// Prune by TTL
err := pruner.PruneMessagesByTTL(sessionKey)

// Compact session by summarizing old messages
err := pruner.CompactSession(sessionKey)
```

**Pruning Strategies:**
- **LRU**: Least Recently Used
- **LFU**: Least Frequently Used
- **TTL**: Time-To-Live based
- **Size**: Largest sessions first
- **Semantic**: Semantic similarity deduplication

## Configuration

### Session Type Limits

```go
// DM (Direct Message) sessions
DMPreserveCount: 100  // Keep last 100 messages

// Group chat sessions
GroupPreserveCount: 50  // Keep last 50 messages
```

### TTL Configuration

```go
// Default message TTL
DefaultMessageTTL: 24 * time.Hour

// Per-message TTL (in Message.Metadata)
msg.Metadata["ttl"] = 2 * time.Hour
```

### Cache Configuration

```go
cache := session.NewCache(session.CacheConfig{
    MaxSize:      100,              // Max cached sessions
    DefaultTTL:   1 * time.Hour,     // Default entry TTL
    CleanupIntvl: 5 * time.Minute,   // Cleanup interval
})
```

## Integration with Existing Manager

The enhanced features integrate seamlessly with the existing `session.Manager`:

```go
// Create manager
manager, err := session.NewManager("/path/to/sessions")

// Add caching
cache := session.NewCache(session.DefaultCacheConfig())

// Add pruning
pruner := session.NewPruner(manager, session.DefaultPruneConfig())

// Use together
func handleSession(key string) (*session.Session, error) {
    // Try cache first
    if sess, ok := cache.Get(key); ok {
        return sess, nil
    }

    // Load from manager
    sess, err := manager.GetOrCreate(key)
    if err != nil {
        return nil, err
    }

    // Store in cache
    cache.Set(key, sess)

    return sess, nil
}
```

## Metadata Enhancements

Enhanced metadata support for better session management:

```go
// Session metadata
session.Metadata = map[string]interface{}{
    "type":      "dm",           // "dm" or "group"
    "title":     "User Interview",
    "tags":      []string{"work", "hr"},
    "pinned":    false,
    "archived":  false,
    "agent":     "primary",
    "priority":  "high",
}

// Message metadata
msg.Metadata = map[string]interface{}{
    "ttl":        24 * time.Hour,
    "important":  true,
    "category":   "preference",
}
```

## Performance Considerations

### Cache Hit Rate

Monitor cache effectiveness:

```go
stats := cache.Stats()
hitRate := cache.HitRate()
fmt.Printf("Hit rate: %.2f%%\n", hitRate)
```

### Memory Usage

Estimate and control memory usage:

```go
// Get estimated size
size := cache.estimateSessionSize(session)

// Prune by size
evicted := cache.PruneBySize(maxTotalSize)
```

### Pruning Statistics

Track pruning operations:

```go
stats := pruner.GetStats()
fmt.Printf("Total prunes: %d\n", stats.TotalPrunes)
fmt.Printf("Messages pruned: %d\n", stats.MessagesPruned)
fmt.Printf("Tokens reclaimed: %d\n", stats.TokensReclaimed)
```

## Best Practices

1. **Always close cache when done**:
   ```go
   defer cache.Close()
   ```

2. **Use appropriate TTLs**:
   - Shorter TTL for high-volume channels
   - Longer TTL for important conversations

3. **Configure limits based on usage**:
   - Higher limits for DM conversations
   - Lower limits for group chats

4. **Monitor statistics**:
   - Track cache hit rates
   - Monitor pruning frequency
   - Adjust limits based on patterns

5. **Backward Compatibility**:
   - Existing JSONL sessions continue to work
   - New features are opt-in via configuration

## Future Enhancements

1. Semantic similarity for deduplication
2. Automatic summarization with LLM
3. Machine learning-based TTL prediction
4. Cross-session context sharing
5. Session templates and presets

## Migration Guide

To migrate existing sessions to tree structure:

```go
// Load existing session
session, err := manager.GetOrCreate("existing-key")

// Create tree structure
tree, err := session.NewSessionTree(session)

// Create branches as needed
branchID, err := tree.CreateBranch(
    tree.rootID,
    newSession,
    "experiment",
    "user",
)
```

## Testing

Run tests with:

```bash
go test ./session/...
```

Key test files:
- `manager_test.go` - Basic manager tests
- `tree_test.go` - Tree structure tests
- `cache_test.go` - Cache behavior tests
- `prune_test.go` - Pruning strategy tests
