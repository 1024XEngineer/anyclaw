#!/usr/bin/env sh
set -eu

DATA_DIR="${DATA_DIR:-/data}"
OUTPUT_DIR="${OUTPUT_DIR:-/backups/registry}"
NAME="${NAME:-$(date -u +%Y%m%d-%H%M%S)}"
BACKUP_DIR="$OUTPUT_DIR/$NAME"
DB_PATH="$DATA_DIR/registry.db"

if [ ! -f "$DB_PATH" ]; then
  echo "Registry database not found: $DB_PATH" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"

if command -v sqlite3 >/dev/null 2>&1; then
  sqlite3 "$DB_PATH" ".backup '$BACKUP_DIR/registry.db'"
  METHOD="sqlite3 .backup"
else
  cp "$DB_PATH" "$BACKUP_DIR/registry.db"
  METHOD="file-copy fallback"
fi

if [ -d "$DATA_DIR/packages" ]; then
  cp -a "$DATA_DIR/packages" "$BACKUP_DIR/packages"
fi
if [ -d "$DATA_DIR/audit" ]; then
  cp -a "$DATA_DIR/audit" "$BACKUP_DIR/audit"
fi

cat > "$BACKUP_DIR/manifest.json" <<EOF
{
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "source_data_dir": "$DATA_DIR",
  "backup_dir": "$BACKUP_DIR",
  "sqlite_backup_method": "$METHOD"
}
EOF

echo "Registry backup ready: $BACKUP_DIR"
