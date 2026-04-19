package sqlite

import (
	"context"
	"strings"
	"testing"
)

func TestQueryBuilderBuildCheckedAllowsSafeIdentifiers(t *testing.T) {
	query, args, err := Query("test").
		Select("id", "name", "test.*").
		Where("id = ?", 1).
		OrderBy("name desc").
		Limit(5).
		BuildChecked()
	if err != nil {
		t.Fatalf("BuildChecked: %v", err)
	}

	expected := "SELECT id, name, test.* FROM test WHERE id = ? ORDER BY name DESC LIMIT 5"
	if query != expected {
		t.Fatalf("expected query %q, got %q", expected, query)
	}
	if len(args) != 1 || args[0] != 1 {
		t.Fatalf("expected args [1], got %#v", args)
	}
}

func TestQueryBuilderBuildCheckedRejectsUnsafeIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		builder *QueryBuilder
	}{
		{
			name:    "unsafe table",
			builder: Query("test; DROP TABLE test"),
		},
		{
			name:    "unsafe column",
			builder: Query("test").Select("name; DROP TABLE test"),
		},
		{
			name:    "unsafe order by",
			builder: Query("test").OrderBy("name desc; DROP TABLE test"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := tc.builder.BuildChecked(); err == nil {
				t.Fatal("expected BuildChecked to reject unsafe identifier")
			}
		})
	}
}

func TestCRUDHelpersRejectUnsafeIdentifiers(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))
	ctx := context.Background()

	if _, err := db.Insert(ctx, "test; DROP TABLE test", map[string]any{"name": "unsafe"}); err == nil {
		t.Fatal("expected Insert to reject unsafe table name")
	}

	if _, err := db.Update(ctx, "test", map[string]any{"name = 'unsafe'": "value"}, "id = ?", 1); err == nil {
		t.Fatal("expected Update to reject unsafe column name")
	}

	if _, err := db.Upsert(ctx, "test", map[string]any{"name": "safe"}, []string{"id); DROP TABLE test; --"}); err == nil {
		t.Fatal("expected Upsert to reject unsafe conflict column")
	}

	if _, err := db.Delete(ctx, "test; DROP TABLE test", "id = ?", 1); err == nil {
		t.Fatal("expected Delete to reject unsafe table name")
	}

	if _, err := db.Get(ctx, "test", []string{"name; DROP TABLE test"}, "id = ?", 1); err == nil {
		t.Fatal("expected Get to reject unsafe column name")
	}

	if _, err := db.List(ctx, "test", []string{"name; DROP TABLE test"}, ""); err == nil {
		t.Fatal("expected List to reject unsafe column name")
	}

	if _, err := db.Count(ctx, "test; DROP TABLE test", ""); err == nil {
		t.Fatal("expected Count to reject unsafe table name")
	}
}

func TestCRUDHelpersPreserveSafeOperations(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))
	ctx := context.Background()

	if _, err := db.Insert(ctx, "test", map[string]any{"name": "safe", "value": "ok"}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	row, err := db.Get(ctx, "test", []string{"id", "name", "value"}, "name = ?", "safe")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got, _ := row["name"].(string); got != "safe" {
		t.Fatalf("expected name safe, got %#v", row["name"])
	}

	rows, err := db.List(ctx, "test", []string{"id", "name", "value"}, "name = ?", "safe")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	count, err := db.Count(ctx, "test", "name = ?", "safe")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestInvalidIdentifierErrorsMentionIdentifier(t *testing.T) {
	if _, err := sanitizeIdentifier("test-name"); err == nil || !strings.Contains(err.Error(), "invalid identifier") {
		t.Fatalf("expected invalid identifier error, got %v", err)
	}
}
