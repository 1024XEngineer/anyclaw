package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileType string

const (
	FileAgents    FileType = "AGENTS.md"
	FileSoul      FileType = "SOUL.md"
	FileTools     FileType = "TOOLS.md"
	FileIdentity  FileType = "IDENTITY.md"
	FileUser      FileType = "USER.md"
	FileHeartbeat FileType = "HEARTBEAT.md"
	FileBootstrap FileType = "BOOTSTRAP.md"
	FileRules     FileType = "RULES.md"
	FileMemory    FileType = "MEMORY.md"
	FileSkills    FileType = "SKILLS.md"
	FileCommands  FileType = "COMMANDS.md"
	FileCustom    FileType = "custom"
)

type FileEntry struct {
	Type     FileType
	Path     string
	Content  string
	LastMod  time.Time
	Size     int64
	Checksum uint32
}

type ChangeEvent struct {
	Type    FileType
	Path    string
	OldSize int64
	NewSize int64
	Action  string // "created", "modified", "deleted"
	Time    time.Time
}

type ChangeHandler func(event ChangeEvent)

type Watcher struct {
	mu       sync.RWMutex
	files    map[FileType]*FileEntry
	handlers []ChangeHandler
	interval time.Duration
	stopCh   chan struct{}
	running  bool
	baseDir  string
}

type WatcherConfig struct {
	BaseDir      string
	PollInterval time.Duration
	AutoLoad     bool
	Files        []FileType
	OnChange     ChangeHandler
}

func DefaultWatcherConfig(baseDir string) WatcherConfig {
	return WatcherConfig{
		BaseDir:      baseDir,
		PollInterval: 2 * time.Second,
		AutoLoad:     true,
		Files: []FileType{
			FileAgents, FileSoul, FileTools,
			FileIdentity, FileUser, FileHeartbeat,
			FileBootstrap, FileRules,
			FileMemory, FileSkills, FileCommands,
		},
	}
}

func NewWatcher(cfg WatcherConfig) *Watcher {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = "."
	}

	w := &Watcher{
		files:    make(map[FileType]*FileEntry),
		interval: cfg.PollInterval,
		stopCh:   make(chan struct{}),
		baseDir:  cfg.BaseDir,
	}

	if cfg.OnChange != nil {
		w.handlers = append(w.handlers, cfg.OnChange)
	}

	if cfg.AutoLoad {
		for _, ft := range cfg.Files {
			w.loadFile(ft)
		}
	}

	return w
}

func (w *Watcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("bootstrap: watcher already running")
	}
	w.running = true
	w.mu.Unlock()

	go w.watchLoop()
	return nil
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}
	w.running = false
	close(w.stopCh)
	w.stopCh = make(chan struct{})
}

func (w *Watcher) Get(ft FileType) (*FileEntry, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entry, ok := w.files[ft]
	if !ok {
		return nil, false
	}
	return entry, true
}

func (w *Watcher) GetContent(ft FileType) (string, bool) {
	entry, ok := w.Get(ft)
	if !ok {
		return "", false
	}
	return entry.Content, true
}

func (w *Watcher) GetAll() map[FileType]*FileEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make(map[FileType]*FileEntry, len(w.files))
	for k, v := range w.files {
		result[k] = v
	}
	return result
}

func (w *Watcher) Reload(ft FileType) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.loadFileLocked(ft)
}

func (w *Watcher) ReloadAll() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var lastErr error
	for ft := range w.files {
		if err := w.loadFileLocked(ft); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (w *Watcher) OnChange(handler ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, handler)
}

func (w *Watcher) watchLoop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkChanges()
		}
	}
}

func (w *Watcher) checkChanges() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for ft, entry := range w.files {
		info, err := os.Stat(entry.Path)
		if err != nil {
			if os.IsNotExist(err) {
				w.notify(ChangeEvent{
					Type:    ft,
					Path:    entry.Path,
					OldSize: entry.Size,
					NewSize: 0,
					Action:  "deleted",
					Time:    time.Now(),
				})
				delete(w.files, ft)
			}
			continue
		}

		content, err := os.ReadFile(entry.Path)
		if err != nil {
			continue
		}

		newContent := string(content)
		newChecksum := simpleChecksum(newContent)
		if newContent == entry.Content {
			entry.LastMod = info.ModTime()
			entry.Size = info.Size()
			entry.Checksum = newChecksum
			continue
		}
		if info.ModTime() == entry.LastMod && info.Size() == entry.Size && newChecksum == entry.Checksum {
			continue
		}

		oldSize := entry.Size
		entry.Content = newContent
		entry.LastMod = info.ModTime()
		entry.Size = info.Size()
		entry.Checksum = newChecksum

		w.notify(ChangeEvent{
			Type:    ft,
			Path:    entry.Path,
			OldSize: oldSize,
			NewSize: info.Size(),
			Action:  "modified",
			Time:    time.Now(),
		})
	}

	// Check for new files
	for _, ft := range []FileType{
		FileAgents, FileSoul, FileTools,
		FileIdentity, FileUser, FileHeartbeat,
		FileBootstrap, FileRules,
		FileMemory, FileSkills, FileCommands,
	} {
		if _, exists := w.files[ft]; exists {
			continue
		}

		path := filepath.Join(w.baseDir, string(ft))
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		entry := &FileEntry{
			Type:     ft,
			Path:     path,
			Content:  string(content),
			LastMod:  info.ModTime(),
			Size:     info.Size(),
			Checksum: simpleChecksum(string(content)),
		}
		w.files[ft] = entry

		w.notify(ChangeEvent{
			Type:    ft,
			Path:    path,
			OldSize: 0,
			NewSize: info.Size(),
			Action:  "created",
			Time:    time.Now(),
		})
	}
}

func (w *Watcher) loadFile(ft FileType) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.loadFileLocked(ft)
}

func (w *Watcher) loadFileLocked(ft FileType) error {
	path := filepath.Join(w.baseDir, string(ft))

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("bootstrap: stat %s: %w", ft, err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("bootstrap: read %s: %w", ft, err)
	}

	w.files[ft] = &FileEntry{
		Type:     ft,
		Path:     path,
		Content:  string(content),
		LastMod:  info.ModTime(),
		Size:     info.Size(),
		Checksum: simpleChecksum(string(content)),
	}

	return nil
}

func (w *Watcher) notify(event ChangeEvent) {
	for _, handler := range w.handlers {
		handler(event)
	}
}

func simpleChecksum(s string) uint32 {
	var sum uint32
	for i := 0; i < len(s); i++ {
		sum = sum*31 + uint32(s[i])
	}
	return sum
}

type FileLoader struct {
	mu      sync.RWMutex
	entries map[FileType]*FileEntry
	baseDir string
}

func NewFileLoader(baseDir string) *FileLoader {
	return &FileLoader{
		entries: make(map[FileType]*FileEntry),
		baseDir: baseDir,
	}
}

func (l *FileLoader) Load(ft FileType) (*FileEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, ok := l.entries[ft]; ok {
		return entry, nil
	}

	path := filepath.Join(l.baseDir, string(ft))
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("bootstrap: load %s: %w", ft, err)
	}

	info, _ := os.Stat(path)
	entry := &FileEntry{
		Type:     ft,
		Path:     path,
		Content:  string(content),
		LastMod:  info.ModTime(),
		Size:     info.Size(),
		Checksum: simpleChecksum(string(content)),
	}
	l.entries[ft] = entry

	return entry, nil
}

func (l *FileLoader) LoadAll(types []FileType) (map[FileType]*FileEntry, error) {
	result := make(map[FileType]*FileEntry)
	for _, ft := range types {
		entry, err := l.Load(ft)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			result[ft] = entry
		}
	}
	return result, nil
}

func (l *FileLoader) Get(ft FileType) (*FileEntry, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	entry, ok := l.entries[ft]
	return entry, ok
}

func (l *FileLoader) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make(map[FileType]*FileEntry)
}
