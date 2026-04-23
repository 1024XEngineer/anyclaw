package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
)

type indexMetaStore struct {
	db        *sql.DB
	tableName string
}

func newIndexMetaStore(db *sql.DB, tableName string) *indexMetaStore {
	if strings.TrimSpace(tableName) == "" {
		tableName = "vector_index_meta"
	}

	return &indexMetaStore{
		db:        db,
		tableName: tableName,
	}
}

func (s *indexMetaStore) Init(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("metadata db is required")
	}
	if err := validateIdentifier("metadata table", s.tableName); err != nil {
		return err
	}

	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		name TEXT PRIMARY KEY,
		table_name TEXT NOT NULL,
		dimensions INTEGER NOT NULL,
		distance TEXT NOT NULL,
		metadata TEXT,
		aux_columns TEXT,
		status TEXT NOT NULL,
		vector_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		error TEXT
	)`, s.tableName)

	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *indexMetaStore) Load(ctx context.Context) (map[string]*IndexInfo, error) {
	if s.db == nil {
		return nil, fmt.Errorf("metadata db is required")
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT name, table_name, dimensions, distance, metadata, aux_columns, status, vector_count, created_at, updated_at, error FROM %s",
		s.tableName,
	))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := make(map[string]*IndexInfo)
	for rows.Next() {
		info, err := scanIndexInfo(rows)
		if err != nil {
			return nil, err
		}
		indexes[info.Name] = info
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return indexes, nil
}

func (s *indexMetaStore) Save(ctx context.Context, info *IndexInfo) error {
	if s.db == nil {
		return fmt.Errorf("metadata db is required")
	}
	if info == nil {
		return fmt.Errorf("index info cannot be nil")
	}

	metaJSON, err := json.Marshal(info.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	auxJSON, err := json.Marshal(info.AuxColumns)
	if err != nil {
		return fmt.Errorf("marshal aux columns: %w", err)
	}

	_, err = s.db.ExecContext(ctx, fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (name, table_name, dimensions, distance, metadata, aux_columns, status, vector_count, created_at, updated_at, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.tableName,
	), info.Name, info.TableName, info.Dimensions, info.Distance,
		string(metaJSON), string(auxJSON), string(info.Status), info.VectorCount,
		info.CreatedAt.Format(time.RFC3339), info.UpdatedAt.Format(time.RFC3339), info.Error)

	return err
}

func (s *indexMetaStore) Delete(ctx context.Context, name string) error {
	if s.db == nil {
		return fmt.Errorf("metadata db is required")
	}

	_, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE name = ?", s.tableName), name)
	return err
}

func scanIndexInfo(scanner interface{ Scan(dest ...any) error }) (*IndexInfo, error) {
	var info IndexInfo
	var metaJSON sql.NullString
	var auxJSON sql.NullString
	var status string
	var createdAt string
	var updatedAt string
	var errorText sql.NullString

	err := scanner.Scan(
		&info.Name,
		&info.TableName,
		&info.Dimensions,
		&info.Distance,
		&metaJSON,
		&auxJSON,
		&status,
		&info.VectorCount,
		&createdAt,
		&updatedAt,
		&errorText,
	)
	if err != nil {
		return nil, err
	}

	info.Status = Status(status)
	if metaJSON.Valid && metaJSON.String != "" {
		if err := json.Unmarshal([]byte(metaJSON.String), &info.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	if auxJSON.Valid && auxJSON.String != "" {
		if err := json.Unmarshal([]byte(auxJSON.String), &info.AuxColumns); err != nil {
			return nil, fmt.Errorf("unmarshal aux columns: %w", err)
		}
	}
	if errorText.Valid {
		info.Error = errorText.String
	}

	if info.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if info.UpdatedAt, err = parseTimestamp(updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &info, nil
}

func parseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func validateIdentifier(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s cannot be empty", kind)
	}

	for i, r := range value {
		if r > unicode.MaxASCII {
			return fmt.Errorf("invalid %s %q", kind, value)
		}
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return fmt.Errorf("invalid %s %q", kind, value)
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Errorf("invalid %s %q", kind, value)
		}
	}

	return nil
}
