package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func RegisterQMDTools(r *Registry, opts BuiltinOptions) {
	if opts.QMDClient == nil {
		return
	}

	r.RegisterTool(
		"qmd",
		"Query and manage structured data in the QMD in-memory data store. Supports: list_tables, list, get, query, insert, update, delete, count",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{"type": "string", "description": "Action: list_tables, list, get, query, insert, update, delete, count"},
				"table":  map[string]string{"type": "string", "description": "Table name (required for all actions except list_tables)"},
				"id":     map[string]string{"type": "string", "description": "Record ID (required for get, update, delete)"},
				"field":  map[string]string{"type": "string", "description": "Field name to filter on (required for query)"},
				"value":  map[string]string{"type": "string", "description": "Value to match (required for query)"},
				"data":   map[string]string{"type": "object", "description": "Record data object (required for insert, update)"},
				"limit":  map[string]string{"type": "number", "description": "Maximum number of records to return (default: 50)"},
			},
			"required": []string{"action"},
		},
		func(ctx context.Context, input map[string]any) (string, error) {
			return QMDTool(ctx, input, opts.QMDClient)
		},
	)
}

func QMDTool(ctx context.Context, input map[string]any, client QMDClient) (string, error) {
	if client == nil {
		return "", fmt.Errorf("QMD structured data store is not available")
	}

	action, _ := input["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return "", fmt.Errorf("action is required: list_tables, list, get, query, insert, update, delete, count")
	}

	table, _ := input["table"].(string)
	table = strings.TrimSpace(table)

	switch action {
	case "list_tables":
		return qmdListTables(ctx, client)
	case "list":
		return qmdList(ctx, client, table, input)
	case "get":
		return qmdGet(ctx, client, table, input)
	case "query":
		return qmdQuery(ctx, client, table, input)
	case "insert":
		return qmdInsert(ctx, client, table, input)
	case "update":
		return qmdUpdate(ctx, client, table, input)
	case "delete":
		return qmdDelete(ctx, client, table, input)
	case "count":
		return qmdCount(ctx, client, table)
	default:
		return "", fmt.Errorf("unknown action %q; use: list_tables, list, get, query, insert, update, delete, count", action)
	}
}

func qmdListTables(ctx context.Context, client QMDClient) (string, error) {
	tables, err := client.ListTables(ctx)
	if err != nil {
		return "", err
	}
	if len(tables) == 0 {
		return "No QMD tables found", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d QMD table(s)\n\n", len(tables)))
	for _, t := range tables {
		sb.WriteString(fmt.Sprintf("- %s (%d rows, %d columns)\n", t.Name, t.RowCount, t.Columns))
	}
	return sb.String(), nil
}

func qmdList(ctx context.Context, client QMDClient, table string, input map[string]any) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for list action")
	}
	limit := 50
	if v, ok := input["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	records, err := client.List(ctx, table, limit)
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return fmt.Sprintf("No records in table %q", table), nil
	}
	return formatRecords(table, records), nil
}

func qmdGet(ctx context.Context, client QMDClient, table string, input map[string]any) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for get action")
	}
	id, _ := input["id"].(string)
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("id is required for get action")
	}
	record, err := client.Get(ctx, table, id)
	if err != nil {
		return "", err
	}
	return formatRecord(table, record), nil
}

func qmdQuery(ctx context.Context, client QMDClient, table string, input map[string]any) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for query action")
	}
	field, _ := input["field"].(string)
	if strings.TrimSpace(field) == "" {
		return "", fmt.Errorf("field is required for query action")
	}
	value := input["value"]
	if value == nil {
		return "", fmt.Errorf("value is required for query action")
	}
	limit := 50
	if v, ok := input["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	records, err := client.Query(ctx, table, field, value, limit)
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return fmt.Sprintf("No records found in table %q where %s = %v", table, field, value), nil
	}
	return formatRecords(table, records), nil
}

func qmdInsert(ctx context.Context, client QMDClient, table string, input map[string]any) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for insert action")
	}
	data, ok := input["data"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("data (object) is required for insert action")
	}
	id, _ := input["id"].(string)
	if strings.TrimSpace(id) == "" {
		id = newQMDRecordID()
	}
	record := map[string]any{"id": id}
	for k, v := range data {
		record[k] = v
	}
	if err := client.Insert(ctx, table, record); err != nil {
		return "", err
	}
	return fmt.Sprintf("Inserted record %q into table %q", id, table), nil
}

func qmdUpdate(ctx context.Context, client QMDClient, table string, input map[string]any) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for update action")
	}
	id, _ := input["id"].(string)
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("id is required for update action")
	}
	data, ok := input["data"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("data (object) is required for update action")
	}
	record := map[string]any{"id": id}
	for k, v := range data {
		record[k] = v
	}
	if err := client.Update(ctx, table, record); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated record %q in table %q", id, table), nil
}

func qmdDelete(ctx context.Context, client QMDClient, table string, input map[string]any) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for delete action")
	}
	id, _ := input["id"].(string)
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("id is required for delete action")
	}
	if err := client.Delete(ctx, table, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Deleted record %q from table %q", id, table), nil
}

func qmdCount(ctx context.Context, client QMDClient, table string) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table is required for count action")
	}
	count, err := client.Count(ctx, table)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Table %q has %d record(s)", table, count), nil
}

func formatRecord(table string, record map[string]any) string {
	data, _ := json.MarshalIndent(record, "", "  ")
	return fmt.Sprintf("Table: %s\n\n%s", table, string(data))
}

func newQMDRecordID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "rec-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("rec-%d", time.Now().UnixNano())
}

func formatRecords(table string, records []map[string]any) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Table: %s (%d records)\n\n", table, len(records)))
	for i, r := range records {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(formatRecord(table, r))
	}
	return sb.String()
}

func parseValue(v any) any {
	switch s := v.(type) {
	case string:
		if strings.ToLower(s) == "true" {
			return true
		}
		if strings.ToLower(s) == "false" {
			return false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	default:
		return v
	}
}
