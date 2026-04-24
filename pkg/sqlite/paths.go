package sqlite

import (
	"path/filepath"
	"strings"
)

// SidecarDir returns a sibling directory next to the configured SQLite file.
// For in-memory DSNs, it returns an empty string.
func (db *DB) SidecarDir(name string) string {
	if db == nil {
		return ""
	}

	dsn := strings.TrimSpace(db.cfg.DSN)
	if dsn == "" || dsn == ":memory:" || strings.Contains(dsn, "mode=memory") {
		return ""
	}

	if strings.HasPrefix(dsn, "file:") {
		dsn = strings.TrimPrefix(dsn, "file:")
	}
	if idx := strings.Index(dsn, "?"); idx >= 0 {
		dsn = dsn[:idx]
	}
	if dsn == "" {
		return ""
	}

	clean := filepath.Clean(dsn)
	base := filepath.Base(clean)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = base
	}
	if name == "" {
		return filepath.Join(filepath.Dir(clean), stem)
	}

	return filepath.Join(filepath.Dir(clean), stem+"."+name)
}
