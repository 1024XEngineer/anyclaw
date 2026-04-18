package runtime

import (
	"context"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/qmd"
)

type qmdAdapter struct {
	client *qmd.Client
}

func (a *qmdAdapter) CreateTable(ctx context.Context, name string, columns []string) error {
	return a.client.CreateTable(ctx, name, columns)
}

func (a *qmdAdapter) Insert(ctx context.Context, table string, record map[string]any) error {
	id, _ := record["id"].(string)
	r := &qmd.Record{ID: id, Data: record}
	return a.client.Insert(ctx, table, r)
}

func (a *qmdAdapter) Get(ctx context.Context, table, id string) (map[string]any, error) {
	r, err := a.client.Get(ctx, table, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": r.ID}
	for k, v := range r.Data {
		out[k] = v
	}
	return out, nil
}

func (a *qmdAdapter) Update(ctx context.Context, table string, record map[string]any) error {
	id, _ := record["id"].(string)
	r := &qmd.Record{ID: id, Data: record}
	return a.client.Update(ctx, table, r)
}

func (a *qmdAdapter) Delete(ctx context.Context, table, id string) error {
	return a.client.Delete(ctx, table, id)
}

func (a *qmdAdapter) List(ctx context.Context, table string, limit int) ([]map[string]any, error) {
	records, err := a.client.List(ctx, table, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(records))
	for i, r := range records {
		m := map[string]any{"id": r.ID}
		for k, v := range r.Data {
			m[k] = v
		}
		out[i] = m
	}
	return out, nil
}

func (a *qmdAdapter) Query(ctx context.Context, table, field string, value any, limit int) ([]map[string]any, error) {
	records, err := a.client.Query(ctx, table, field, value, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(records))
	for i, r := range records {
		m := map[string]any{"id": r.ID}
		for k, v := range r.Data {
			m[k] = v
		}
		out[i] = m
	}
	return out, nil
}

func (a *qmdAdapter) ListTables(ctx context.Context) ([]tools.TableStat, error) {
	tables, err := a.client.ListTables(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tools.TableStat, len(tables))
	for i, t := range tables {
		out[i] = tools.TableStat{Name: t.Name, RowCount: t.RowCount, Columns: t.Columns}
	}
	return out, nil
}

func (a *qmdAdapter) Count(ctx context.Context, table string) (int, error) {
	return a.client.Count(ctx, table)
}
