#!/usr/bin/env sh
set -eu

BACKUP_DIR="${BACKUP_DIR:?BACKUP_DIR is required}"
TARGET_DATA_DIR="${TARGET_DATA_DIR:-/data}"
FORCE="${FORCE:-false}"

if [ ! -f "$BACKUP_DIR/registry.db" ]; then
  echo "Backup database not found: $BACKUP_DIR/registry.db" >&2
  exit 1
fi

if [ -d "$TARGET_DATA_DIR" ] && [ "$(find "$TARGET_DATA_DIR" -mindepth 1 -maxdepth 1 | wc -l)" -gt 0 ] && [ "$FORCE" != "true" ]; then
  echo "Target data dir is not empty. Set FORCE=true to replace it: $TARGET_DATA_DIR" >&2
  exit 1
fi

rm -rf "$TARGET_DATA_DIR"
mkdir -p "$TARGET_DATA_DIR"
cp "$BACKUP_DIR/registry.db" "$TARGET_DATA_DIR/registry.db"

if [ -d "$BACKUP_DIR/packages" ]; then
  cp -a "$BACKUP_DIR/packages" "$TARGET_DATA_DIR/packages"
fi
if [ -d "$BACKUP_DIR/audit" ]; then
  cp -a "$BACKUP_DIR/audit" "$TARGET_DATA_DIR/audit"
fi

echo "Registry restore ready: $TARGET_DATA_DIR"
