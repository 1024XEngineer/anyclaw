package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	readFile = os.ReadFile
	statFile = os.Stat
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
	Type    FileType
	Path    string
	Content string
	LastMod time.Time
	Size    int64
}

type ChangeAction string

const (
	ActionCreated  ChangeAction = "created"
	ActionModified ChangeAction = "modified"
	ActionDeleted  ChangeAction = "deleted"
)

type ChangeEvent struct {
	Type    FileType
	Path    string
	OldSize int64
	NewSize int64
	Action  ChangeAction
	Time    time.Time
}

type ChangeHandler func(event ChangeEvent)

type Watcher struct {
	mu       sync.RWMutex
	files    map[FileType]*FileEntry
	handlers []ChangeHandler
	watchSet []FileType
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
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
		Files:        defaultFileTypes(),
	}
}

func NewWatcher(cfg WatcherConfig) *Watcher {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = "."
	}

	watchSet := normalizeWatchSet(cfg.Files)

	w := &Watcher{
		files:    make(map[FileType]*FileEntry),
		watchSet: watchSet,
		interval: cfg.PollInterval,
		baseDir:  cfg.BaseDir,
	}

	if cfg.OnChange != nil {
		w.handlers = append(w.handlers, cfg.OnChange)
	}

	if cfg.AutoLoad {
		for _, ft := range watchSet {
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
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	w.stopCh = stopCh
	w.doneCh = doneCh
	w.running = true
	w.mu.Unlock()

	go w.watchLoop(stopCh, doneCh)
	return nil
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	stopCh := w.stopCh
	doneCh := w.doneCh
	w.running = false
	w.stopCh = nil
	w.doneCh = nil
	w.mu.Unlock()

	close(stopCh)
	<-doneCh
}

func (w *Watcher) Get(ft FileType) (*FileEntry, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entry, ok := w.files[ft]
	if !ok {
		return nil, false
	}
	return cloneFileEntry(entry), true
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
		result[k] = cloneFileEntry(v)
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

	var errs []error
	for _, ft := range w.watchSet {
		if err := w.loadFileLocked(ft); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (w *Watcher) OnChange(handler ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, handler)
}

func (w *Watcher) watchLoop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	defer close(doneCh)

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			w.checkChanges()
		}
	}
}

func (w *Watcher) checkChanges() {
	w.mu.Lock()
	events := make([]ChangeEvent, 0)

	for ft, entry := range w.files {
		info, err := statFile(entry.Path)
		if err != nil {
			if os.IsNotExist(err) {
				events = append(events, ChangeEvent{
					Type:    ft,
					Path:    entry.Path,
					OldSize: entry.Size,
					NewSize: 0,
					Action:  ActionDeleted,
					Time:    time.Now(),
				})
				delete(w.files, ft)
			}
			continue
		}
		if info.ModTime() == entry.LastMod && info.Size() == entry.Size && time.Since(entry.LastMod) > w.metadataGraceWindow() {
			continue
		}

		content, err := readFile(entry.Path)
		if err != nil {
			continue
		}

		newContent := string(content)
		if newContent == entry.Content {
			entry.LastMod = info.ModTime()
			entry.Size = info.Size()
			continue
		}

		oldSize := entry.Size
		entry.Content = newContent
		entry.LastMod = info.ModTime()
		entry.Size = info.Size()

		events = append(events, ChangeEvent{
			Type:    ft,
			Path:    entry.Path,
			OldSize: oldSize,
			NewSize: info.Size(),
			Action:  ActionModified,
			Time:    time.Now(),
		})
	}

	for _, ft := range w.watchSet {
		if _, exists := w.files[ft]; exists {
			continue
		}

		path := filepath.Join(w.baseDir, string(ft))
		info, err := statFile(path)
		if err != nil {
			continue
		}

		content, err := readFile(path)
		if err != nil {
			continue
		}

		entry := &FileEntry{
			Type:    ft,
			Path:    path,
			Content: string(content),
			LastMod: info.ModTime(),
			Size:    info.Size(),
		}
		w.files[ft] = entry

		events = append(events, ChangeEvent{
			Type:    ft,
			Path:    path,
			OldSize: 0,
			NewSize: info.Size(),
			Action:  ActionCreated,
			Time:    time.Now(),
		})
	}

	w.mu.Unlock()

	for _, event := range events {
		w.notify(event)
	}
}

func (w *Watcher) loadFile(ft FileType) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.loadFileLocked(ft)
}

func (w *Watcher) loadFileLocked(ft FileType) error {
	path := filepath.Join(w.baseDir, string(ft))

	info, err := statFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			delete(w.files, ft)
			return nil
		}
		return fmt.Errorf("bootstrap: stat %s: %w", ft, err)
	}

	content, err := readFile(path)
	if err != nil {
		return fmt.Errorf("bootstrap: read %s: %w", ft, err)
	}

	w.files[ft] = &FileEntry{
		Type:    ft,
		Path:    path,
		Content: string(content),
		LastMod: info.ModTime(),
		Size:    info.Size(),
	}

	return nil
}

func (w *Watcher) notify(event ChangeEvent) {
	handlers := w.snapshotHandlers()
	for _, handler := range handlers {
		handler := handler
		event := event
		go handler(event)
	}
}

func (w *Watcher) metadataGraceWindow() time.Duration {
	if w.interval > time.Second {
		return w.interval
	}
	return time.Second
}

func (w *Watcher) snapshotHandlers() []ChangeHandler {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(w.handlers) == 0 {
		return nil
	}

	handlers := make([]ChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)
	return handlers
}

func cloneFileEntry(entry *FileEntry) *FileEntry {
	if entry == nil {
		return nil
	}

	cloned := *entry
	return &cloned
}

func defaultFileTypes() []FileType {
	return []FileType{
		FileAgents, FileSoul, FileTools,
		FileIdentity, FileUser, FileHeartbeat,
		FileBootstrap, FileRules,
		FileMemory, FileSkills, FileCommands,
	}
}

func normalizeWatchSet(files []FileType) []FileType {
	if len(files) == 0 {
		files = defaultFileTypes()
	}

	seen := make(map[FileType]struct{}, len(files))
	result := make([]FileType, 0, len(files))
	for _, ft := range files {
		if ft == "" {
			continue
		}
		if _, ok := seen[ft]; ok {
			continue
		}
		seen[ft] = struct{}{}
		result = append(result, ft)
	}
	return result
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
	content, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("bootstrap: load %s: %w", ft, err)
	}

	info, err := statFile(path)
	entry := &FileEntry{
		Type:    ft,
		Path:    path,
		Content: string(content),
	}
	if err == nil {
		entry.LastMod = info.ModTime()
		entry.Size = info.Size()
	} else if os.IsNotExist(err) {
		entry.Size = int64(len(content))
	} else {
		return nil, fmt.Errorf("bootstrap: stat %s: %w", ft, err)
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
