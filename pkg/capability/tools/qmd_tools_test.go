package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestQMDToolActions(t *testing.T) {
	client := &stubRichQMDClient{
		tables: []TableStat{{Name: "tasks", RowCount: 2, Columns: 3}},
		record: map[string]any{"id": "1", "name": "demo"},
		records: []map[string]any{
			{"id": "1", "name": "demo"},
			{"id": "2", "name": "test"},
		},
		count: 2,
	}

	listTables, err := QMDTool(context.Background(), map[string]any{"action": "list_tables"}, client)
	if err != nil || !strings.Contains(listTables, "tasks") {
		t.Fatalf("list_tables returned %q, %v", listTables, err)
	}

	list, err := QMDTool(context.Background(), map[string]any{"action": "list", "table": "tasks"}, client)
	if err != nil || !strings.Contains(list, `"name": "demo"`) {
		t.Fatalf("list returned %q, %v", list, err)
	}

	get, err := QMDTool(context.Background(), map[string]any{"action": "get", "table": "tasks", "id": "1"}, client)
	if err != nil || !strings.Contains(get, "Table: tasks") {
		t.Fatalf("get returned %q, %v", get, err)
	}

	query, err := QMDTool(context.Background(), map[string]any{
		"action": "query",
		"table":  "tasks",
		"field":  "name",
		"value":  "demo",
	}, client)
	if err != nil || !strings.Contains(query, `"id": "1"`) {
		t.Fatalf("query returned %q, %v", query, err)
	}

	insert, err := QMDTool(context.Background(), map[string]any{
		"action": "insert",
		"table":  "tasks",
		"id":     "3",
		"data":   map[string]any{"name": "inserted"},
	}, client)
	if err != nil || !strings.Contains(insert, `"3"`) {
		t.Fatalf("insert returned %q, %v", insert, err)
	}

	update, err := QMDTool(context.Background(), map[string]any{
		"action": "update",
		"table":  "tasks",
		"id":     "1",
		"data":   map[string]any{"name": "updated"},
	}, client)
	if err != nil || !strings.Contains(update, `Updated record`) {
		t.Fatalf("update returned %q, %v", update, err)
	}

	del, err := QMDTool(context.Background(), map[string]any{"action": "delete", "table": "tasks", "id": "1"}, client)
	if err != nil || !strings.Contains(del, `Deleted record`) {
		t.Fatalf("delete returned %q, %v", del, err)
	}

	count, err := QMDTool(context.Background(), map[string]any{"action": "count", "table": "tasks"}, client)
	if err != nil || !strings.Contains(count, `2 record`) {
		t.Fatalf("count returned %q, %v", count, err)
	}
}

func TestQMDToolErrors(t *testing.T) {
	client := &stubRichQMDClient{listErr: errors.New("boom")}

	if _, err := QMDTool(context.Background(), map[string]any{}, client); err == nil {
		t.Fatal("expected missing action error")
	}
	if _, err := QMDTool(context.Background(), map[string]any{"action": "unknown"}, client); err == nil {
		t.Fatal("expected unknown action error")
	}
	if _, err := qmdList(context.Background(), client, "", nil); err == nil {
		t.Fatal("expected missing table for list")
	}
	if _, err := qmdGet(context.Background(), client, "tasks", map[string]any{}); err == nil {
		t.Fatal("expected missing id for get")
	}
	if _, err := qmdQuery(context.Background(), client, "tasks", map[string]any{}); err == nil {
		t.Fatal("expected missing field/value for query")
	}
	if _, err := qmdInsert(context.Background(), client, "tasks", map[string]any{}); err == nil {
		t.Fatal("expected missing data for insert")
	}
	if _, err := qmdUpdate(context.Background(), client, "tasks", map[string]any{"data": map[string]any{}}); err == nil {
		t.Fatal("expected missing id for update")
	}
	if _, err := qmdDelete(context.Background(), client, "tasks", map[string]any{}); err == nil {
		t.Fatal("expected missing id for delete")
	}
	if _, err := qmdList(context.Background(), client, "tasks", nil); err == nil {
		t.Fatal("expected list backend error")
	}
}

func TestParseValue(t *testing.T) {
	if got := parseValue("true"); got != true {
		t.Fatalf("expected boolean true, got %#v", got)
	}
	if got := parseValue("42"); got != int64(42) {
		t.Fatalf("expected integer conversion, got %#v", got)
	}
	if got := parseValue("3.14"); got != 3.14 {
		t.Fatalf("expected float conversion, got %#v", got)
	}
	if got := parseValue("demo"); got != "demo" {
		t.Fatalf("expected string passthrough, got %#v", got)
	}
}

func TestRegisterQMDToolsBehavior(t *testing.T) {
	registry := NewRegistry()
	RegisterQMDTools(registry, BuiltinOptions{})
	if _, ok := registry.Get("qmd"); ok {
		t.Fatal("expected QMD tool to be skipped when client is nil")
	}

	client := &stubRichQMDClient{tables: []TableStat{{Name: "tasks", RowCount: 1, Columns: 2}}}
	RegisterQMDTools(registry, BuiltinOptions{QMDClient: client})
	tool, ok := registry.Get("qmd")
	if !ok {
		t.Fatal("expected QMD tool to be registered")
	}
	result, err := tool.Handler(context.Background(), map[string]any{"action": "list_tables"})
	if err != nil || !strings.Contains(result, "tasks") {
		t.Fatalf("registered QMD handler returned %q, %v", result, err)
	}

	emptyClient := &stubRichQMDClient{}
	output, err := qmdListTables(context.Background(), emptyClient)
	if err != nil || output != "No QMD tables found" {
		t.Fatalf("qmdListTables empty returned %q, %v", output, err)
	}
}

type stubRichQMDClient struct {
	tables  []TableStat
	record  map[string]any
	records []map[string]any
	count   int
	listErr error
}

func (s *stubRichQMDClient) CreateTable(context.Context, string, []string) error {
	return nil
}

func (s *stubRichQMDClient) Insert(context.Context, string, map[string]any) error {
	return nil
}

func (s *stubRichQMDClient) Get(context.Context, string, string) (map[string]any, error) {
	return s.record, nil
}

func (s *stubRichQMDClient) Update(context.Context, string, map[string]any) error {
	return nil
}

func (s *stubRichQMDClient) Delete(context.Context, string, string) error {
	return nil
}

func (s *stubRichQMDClient) List(context.Context, string, int) ([]map[string]any, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.records, nil
}

func (s *stubRichQMDClient) Query(context.Context, string, string, any, int) ([]map[string]any, error) {
	return s.records, nil
}

func (s *stubRichQMDClient) ListTables(context.Context) ([]TableStat, error) {
	return s.tables, nil
}

func (s *stubRichQMDClient) Count(context.Context, string) (int, error) {
	return s.count, nil
}
