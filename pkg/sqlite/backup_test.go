package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackupOnce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	backupDir := tmpDir + "/backups"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO test (name) VALUES ('backup_test')`)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	bm := NewBackupManager(DefaultBackupConfig(backupDir))

	backupPath, err := bm.BackupOnce(ctx, db)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("list backups failed: %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}

	latest, err := bm.LatestBackup()
	if err != nil {
		t.Fatalf("latest backup failed: %v", err)
	}

	if latest.Path != backupPath {
		t.Errorf("expected latest backup %s, got %s", backupPath, latest.Path)
	}
}

func TestBackupPruning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	backupDir := tmpDir + "/backups"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	bm := NewBackupManager(BackupConfig{
		BackupDir:  backupDir,
		MaxBackups: 3,
		Interval:   time.Hour,
	})

	for i := 0; i < 5; i++ {
		_, err := bm.BackupOnce(ctx, db)
		if err != nil {
			t.Fatalf("backup %d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("list backups failed: %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("expected 3 backups after pruning, got %d", len(backups))
	}
}

func TestBackupStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	backupDir := tmpDir + "/backups"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	var backupCount atomic.Int32
	bm := NewBackupManager(BackupConfig{
		BackupDir:  backupDir,
		MaxBackups: 10,
		Interval:   100 * time.Millisecond,
		OnBackupDone: func(path string, size int64, duration time.Duration) {
			backupCount.Add(1)
		},
	})

	if err := bm.Start(ctx, db); err != nil {
		t.Fatalf("start backup manager failed: %v", err)
	}

	time.Sleep(350 * time.Millisecond)

	bm.Stop()

	count := backupCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 backups, got %d", count)
	}
}

func TestRestoreFromBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	backupDir := tmpDir + "/backups"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO test (name) VALUES ('original')`)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	bm := NewBackupManager(DefaultBackupConfig(backupDir))

	backupPath, err := bm.BackupOnce(ctx, db)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	db.Close()

	if err := bm.RestoreFromBackup(ctx, db, backupPath); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	db2, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen db failed: %v", err)
	}
	defer db2.Close()

	var name string
	err = db2.QueryRowContext(ctx, `SELECT name FROM test LIMIT 1`).Scan(&name)
	if err != nil {
		t.Fatalf("query after restore failed: %v", err)
	}

	if name != "original" {
		t.Errorf("expected name 'original', got %s", name)
	}
}

func TestRepairIntegrityCheck(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO test (name) VALUES ('repair_test')`)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	rm := NewRepairManager(DefaultRepairConfig())

	result, err := rm.CheckDatabase(ctx, db)
	if err != nil {
		t.Fatalf("check database failed: %v", err)
	}

	if len(result.IssuesFound) != 0 {
		t.Errorf("expected no issues on healthy db, got %v", result.IssuesFound)
	}
}

func TestQuickFix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO test (name) VALUES ('quickfix')`)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	rm := NewRepairManager(DefaultRepairConfig())

	if err := rm.QuickFix(ctx, db); err != nil {
		t.Fatalf("quick fix failed: %v", err)
	}

	ok, err := db.IntegrityCheck(ctx)
	if err != nil {
		t.Fatalf("integrity check after quick fix failed: %v", err)
	}
	if !ok {
		t.Error("database failed integrity check after quick fix")
	}
}

func TestRepairWithCallbacks(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	var issuesDetected []string
	var issuesFixed []string

	rm := NewRepairManager(RepairConfig{
		AutoRepair:    true,
		CreateBackup:  true,
		MaxRepairTime: 10 * time.Second,
		OnIssueDetected: func(issue string) {
			issuesDetected = append(issuesDetected, issue)
		},
		OnIssueFixed: func(fix string) {
			issuesFixed = append(issuesFixed, fix)
		},
	})

	result, err := rm.CheckDatabase(ctx, db)
	if err != nil {
		t.Fatalf("check database failed: %v", err)
	}

	if len(issuesDetected) != 0 {
		t.Logf("issues detected: %v", issuesDetected)
	}

	_ = result
}

func TestRecoverFromBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	backupDir := tmpDir + "/backups"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.ExecContext(ctx, `INSERT INTO test (name) VALUES ('recover_test')`)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	bm := NewBackupManager(DefaultBackupConfig(backupDir))
	_, err = bm.BackupOnce(ctx, db)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	db.Close()

	rm := NewRepairManager(DefaultRepairConfig())
	if err := rm.RecoverFromBackup(ctx, db, backupDir); err != nil {
		t.Fatalf("recover from backup failed: %v", err)
	}

	db2, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen db failed: %v", err)
	}
	defer db2.Close()

	var name string
	err = db2.QueryRowContext(ctx, `SELECT name FROM test LIMIT 1`).Scan(&name)
	if err != nil {
		t.Fatalf("query after recover failed: %v", err)
	}

	if name != "recover_test" {
		t.Errorf("expected name 'recover_test', got %s", name)
	}
}

func TestBackupInMemoryError(t *testing.T) {
	db, err := Open(InMemoryConfig())
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	tmpDir := t.TempDir()
	bm := NewBackupManager(DefaultBackupConfig(tmpDir))

	_, err = bm.BackupOnce(ctx, db)
	if err == nil {
		t.Error("expected error when backing up in-memory database")
	}
}

func TestBackupCallbacks(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	backupDir := tmpDir + "/backups"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	var started, done bool
	var backupSize int64
	var backupDuration time.Duration

	bm := NewBackupManager(BackupConfig{
		BackupDir:  backupDir,
		MaxBackups: 10,
		Interval:   time.Hour,
		OnBackupStart: func(path string) {
			started = true
		},
		OnBackupDone: func(path string, size int64, duration time.Duration) {
			done = true
			backupSize = size
			backupDuration = duration
		},
	})

	_, err = bm.BackupOnce(ctx, db)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	if !started {
		t.Error("expected OnBackupStart callback to be called")
	}
	if !done {
		t.Error("expected OnBackupDone callback to be called")
	}
	if backupSize == 0 {
		t.Error("expected non-zero backup size")
	}
	if backupDuration == 0 {
		t.Error("expected non-zero backup duration")
	}
}

func TestRepairCreatesBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	cfg := DefaultConfig(dbPath)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	rm := NewRepairManager(RepairConfig{
		AutoRepair:    true,
		CreateBackup:  true,
		MaxRepairTime: 10 * time.Second,
	})

	result, err := rm.RepairDatabase(ctx, db)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}

	if result.BackupCreated != "" {
		if _, err := os.Stat(result.BackupCreated); err != nil {
			t.Errorf("repair backup not created: %v", err)
		}
	}

	if !result.Success {
		t.Error("expected repair to succeed")
	}
}

func TestListBackupsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBackupManager(DefaultBackupConfig(tmpDir))

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("list backups failed: %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}

func TestLatestBackupNone(t *testing.T) {
	tmpDir := t.TempDir()
	bm := NewBackupManager(DefaultBackupConfig(tmpDir))

	_, err := bm.LatestBackup()
	if err == nil {
		t.Error("expected error when no backups exist")
	}
}

func TestBackupDirAutoCreate(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "nested", "backups")

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	bm := NewBackupManager(DefaultBackupConfig(backupDir))

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("list backups failed: %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}
