package sqlite

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type BackupConfig struct {
	BackupDir     string
	MaxBackups    int
	Interval      time.Duration
	Compress      bool
	OnBackupStart func(path string)
	OnBackupDone  func(path string, size int64, duration time.Duration)
	OnBackupError func(err error)
}

func DefaultBackupConfig(backupDir string) BackupConfig {
	return BackupConfig{
		BackupDir:  backupDir,
		MaxBackups: 10,
		Interval:   1 * time.Hour,
		Compress:   false,
	}
}

type BackupInfo struct {
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
	Name      string    `json:"name"`
}

type BackupManager struct {
	mu      sync.Mutex
	cfg     BackupConfig
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
	counter int64
}

func NewBackupManager(cfg BackupConfig) *BackupManager {
	return &BackupManager{
		cfg:    cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (bm *BackupManager) Start(ctx context.Context, db *DB) error {
	bm.mu.Lock()
	if bm.running {
		bm.mu.Unlock()
		return fmt.Errorf("sqlite: backup manager already running")
	}
	bm.running = true
	bm.mu.Unlock()

	if err := os.MkdirAll(bm.cfg.BackupDir, 0o755); err != nil {
		return fmt.Errorf("sqlite: create backup dir: %w", err)
	}

	go bm.runLoop(ctx, db)

	return nil
}

func (bm *BackupManager) Stop() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.running {
		return
	}

	close(bm.stopCh)
	bm.running = false
}

func (bm *BackupManager) Wait() {
	<-bm.doneCh
}

func (bm *BackupManager) BackupOnce(ctx context.Context, db *DB) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if err := os.MkdirAll(bm.cfg.BackupDir, 0o755); err != nil {
		return "", fmt.Errorf("sqlite: create backup dir: %w", err)
	}

	start := time.Now()
	bm.counter++
	timestamp := start.Format("20060102_150405")
	backupPath := filepath.Join(bm.cfg.BackupDir, fmt.Sprintf("backup_%s_%03d.db", timestamp, bm.counter))

	if bm.cfg.OnBackupStart != nil {
		bm.cfg.OnBackupStart(backupPath)
	}

	if err := bm.performBackup(ctx, db, backupPath); err != nil {
		if bm.cfg.OnBackupError != nil {
			bm.cfg.OnBackupError(err)
		}
		return "", fmt.Errorf("sqlite: backup: %w", err)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return "", err
	}

	if bm.cfg.OnBackupDone != nil {
		bm.cfg.OnBackupDone(backupPath, info.Size(), time.Since(start))
	}

	if err := bm.pruneOldBackups(); err != nil {
		return backupPath, fmt.Errorf("sqlite: prune backups: %w", err)
	}

	return backupPath, nil
}

func (bm *BackupManager) performBackup(ctx context.Context, db *DB, backupPath string) error {
	if err := db.Checkpoint(ctx, "TRUNCATE"); err != nil {
		return fmt.Errorf("checkpoint before backup: %w", err)
	}

	srcDB := db.DSN()
	if srcDB == "" || srcDB == ":memory:" {
		return fmt.Errorf("sqlite: cannot backup in-memory database")
	}

	srcFile, err := os.Open(srcDB)
	if err != nil {
		return fmt.Errorf("open source db: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy to backup: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}

	return nil
}

func (bm *BackupManager) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(bm.cfg.BackupDir)
	if err != nil {
		return nil, err
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "backup_") || !strings.HasSuffix(name, ".db") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		base := strings.TrimSuffix(strings.TrimPrefix(name, "backup_"), ".db")
		parts := strings.SplitN(base, "_", 3)
		if len(parts) < 2 {
			continue
		}
		tsStr := parts[0] + "_" + parts[1]
		ts, err := time.Parse("20060102_150405", tsStr)
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Path:      filepath.Join(bm.cfg.BackupDir, name),
			Timestamp: ts,
			Size:      info.Size(),
			Name:      name,
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

func (bm *BackupManager) LatestBackup() (*BackupInfo, error) {
	backups, err := bm.ListBackups()
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, fmt.Errorf("sqlite: no backups found")
	}
	return &backups[0], nil
}

func (bm *BackupManager) RestoreFromBackup(ctx context.Context, db *DB, backupPath string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("sqlite: backup file not found: %w", err)
	}

	srcFile, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("sqlite: open backup file: %w", err)
	}
	defer srcFile.Close()

	dstPath := db.DSN()
	if dstPath == "" || dstPath == ":memory:" {
		return fmt.Errorf("sqlite: cannot restore to in-memory database")
	}

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("sqlite: create database file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("sqlite: restore database: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sqlite: sync restored database: %w", err)
	}

	return nil
}

func (bm *BackupManager) pruneOldBackups() error {
	backups, err := bm.ListBackups()
	if err != nil {
		return err
	}

	if len(backups) <= bm.cfg.MaxBackups {
		return nil
	}

	toDelete := backups[bm.cfg.MaxBackups:]
	for _, backup := range toDelete {
		if err := os.Remove(backup.Path); err != nil {
			return fmt.Errorf("remove old backup %s: %w", backup.Path, err)
		}
	}

	return nil
}

func (bm *BackupManager) runLoop(ctx context.Context, db *DB) {
	defer close(bm.doneCh)

	ticker := time.NewTicker(bm.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-bm.stopCh:
			return
		case <-ticker.C:
			if _, err := bm.BackupOnce(ctx, db); err != nil {
				if bm.cfg.OnBackupError != nil {
					bm.cfg.OnBackupError(err)
				}
			}
		}
	}
}
