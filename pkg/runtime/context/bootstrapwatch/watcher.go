package bootstrap

import (
	"errors"
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
	mu           sync.RWMutex
	opsMu        sync.Mutex
	dispatchMu   sync.Mutex
	dispatchTail chan struct{}
	files        map[FileType]*FileEntry
	handlers     []ChangeHandler
	interval     time.Duration
	stopCh       chan struct{}
	running      bool
	baseDir      string
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
		files:        make(map[FileType]*FileEntry),
		interval:     cfg.PollInterval,
		stopCh:       make(chan struct{}),
		baseDir:      cfg.BaseDir,
		dispatchTail: closedSignal(),
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
	w.opsMu.Lock()
	defer w.opsMu.Unlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	return w.loadFileLocked(ft)
}

func (w *Watcher) ReloadAll() error {
	w.opsMu.Lock()
	defer w.opsMu.Unlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	var errs []error
	for ft := range w.files {
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
	w.opsMu.Lock()
	defer w.opsMu.Unlock()

	snapshot, baseDir := w.snapshotFiles()
	candidates := w.scanChanges(snapshot, baseDir)
	events, handlers := w.applyChanges(candidates)
	w.enqueueNotifications(events, handlers)
}

func (w *Watcher) loadFile(ft FileType) {
	w.opsMu.Lock()
	defer w.opsMu.Unlock()

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
		Type:    ft,
		Path:    path,
		Content: string(content),
		LastMod: info.ModTime(),
		Size:    info.Size(),
	}

	return nil
}

func (w *Watcher) metadataGraceWindow() time.Duration {
	if w.interval > time.Second {
		return w.interval
	}
	return time.Second
}

type fileSnapshot struct {
	fileType FileType
	entry    FileEntry
}

type fileChange struct {
	fileType FileType
	entry    *FileEntry
	event    *ChangeEvent
}

func (w *Watcher) snapshotFiles() ([]fileSnapshot, string) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	snapshot := make([]fileSnapshot, 0, len(w.files))
	for ft, entry := range w.files {
		if entry == nil {
			continue
		}
		snapshot = append(snapshot, fileSnapshot{
			fileType: ft,
			entry:    *entry,
		})
	}

	return snapshot, w.baseDir
}

func (w *Watcher) scanChanges(snapshot []fileSnapshot, baseDir string) []fileChange {
	candidates := make([]fileChange, 0, len(snapshot)+len(defaultWatchFileTypes()))
	known := make(map[FileType]struct{}, len(snapshot))

	for _, item := range snapshot {
		known[item.fileType] = struct{}{}

		info, err := os.Stat(item.entry.Path)
		if err != nil {
			if os.IsNotExist(err) {
				candidates = append(candidates, fileChange{
					fileType: item.fileType,
					event: &ChangeEvent{
						Type:    item.fileType,
						Path:    item.entry.Path,
						OldSize: item.entry.Size,
						NewSize: 0,
						Action:  ActionDeleted,
						Time:    time.Now(),
					},
				})
			}
			continue
		}

		// Some filesystems can coalesce rapid writes into the same modtime.
		// Once a file has been stable for a short grace window, stat metadata is
		// enough to skip the more expensive full read.
		if info.ModTime() == item.entry.LastMod && info.Size() == item.entry.Size && time.Since(item.entry.LastMod) > w.metadataGraceWindow() {
			continue
		}

		content, err := os.ReadFile(item.entry.Path)
		if err != nil {
			continue
		}

		updated := item.entry
		updated.LastMod = info.ModTime()
		updated.Size = info.Size()

		newContent := string(content)
		if newContent == item.entry.Content {
			candidates = append(candidates, fileChange{
				fileType: item.fileType,
				entry:    &updated,
			})
			continue
		}

		updated.Content = newContent
		candidates = append(candidates, fileChange{
			fileType: item.fileType,
			entry:    &updated,
			event: &ChangeEvent{
				Type:    item.fileType,
				Path:    item.entry.Path,
				OldSize: item.entry.Size,
				NewSize: info.Size(),
				Action:  ActionModified,
				Time:    time.Now(),
			},
		})
	}

	for _, ft := range defaultWatchFileTypes() {
		if _, exists := known[ft]; exists {
			continue
		}

		path := filepath.Join(baseDir, string(ft))
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		content, err := os.ReadFile(path)
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
		candidates = append(candidates, fileChange{
			fileType: ft,
			entry:    entry,
			event: &ChangeEvent{
				Type:    ft,
				Path:    path,
				OldSize: 0,
				NewSize: info.Size(),
				Action:  ActionCreated,
				Time:    time.Now(),
			},
		})
	}

	return candidates
}

func (w *Watcher) applyChanges(candidates []fileChange) ([]ChangeEvent, []ChangeHandler) {
	if len(candidates) == 0 {
		return nil, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	events := make([]ChangeEvent, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.entry == nil {
			delete(w.files, candidate.fileType)
		} else if current, ok := w.files[candidate.fileType]; ok && current != nil {
			*current = *candidate.entry
		} else {
			entryCopy := *candidate.entry
			w.files[candidate.fileType] = &entryCopy
		}

		if candidate.event != nil {
			events = append(events, *candidate.event)
		}
	}

	if len(events) == 0 || len(w.handlers) == 0 {
		return events, nil
	}

	handlers := append([]ChangeHandler(nil), w.handlers...)
	return events, handlers
}

func (w *Watcher) enqueueNotifications(events []ChangeEvent, handlers []ChangeHandler) {
	if len(events) == 0 || len(handlers) == 0 {
		return
	}

	done := make(chan struct{})

	w.dispatchMu.Lock()
	prev := w.dispatchTail
	w.dispatchTail = done
	w.dispatchMu.Unlock()

	go func() {
		defer close(done)
		<-prev
		for _, event := range events {
			for _, handler := range handlers {
				handler(event)
			}
		}
	}()
}

func defaultWatchFileTypes() []FileType {
	return []FileType{
		FileAgents, FileSoul, FileTools,
		FileIdentity, FileUser, FileHeartbeat,
		FileBootstrap, FileRules,
		FileMemory, FileSkills, FileCommands,
	}
}

func closedSignal() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
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
		Type:    ft,
		Path:    path,
		Content: string(content),
		LastMod: info.ModTime(),
		Size:    info.Size(),
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
