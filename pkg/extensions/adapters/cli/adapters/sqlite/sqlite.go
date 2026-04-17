package sqlite

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	sqlitePath string
}

type Config struct {
	SQLitePath string
}

func NewClient(cfg Config) *Client {
	path := cfg.SQLitePath
	if path == "" {
		path = "sqlite3"
	}
	return &Client{sqlitePath: path}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "Usage: sqlite <db> <sql>", nil
	}

	dbPath := args[0]
	sqlQuery := strings.Join(args[1:], " ")

	query := strings.TrimSpace(sqlQuery)
	if strings.HasPrefix(strings.ToLower(query), "select") ||
		strings.HasPrefix(strings.ToLower(query), "pragma") {
		return c.query(ctx, dbPath, query)
	}

	return c.execute(ctx, dbPath, query)
}

func (c *Client) query(ctx context.Context, dbPath, query string) (string, error) {
	return c.run(ctx, []string{"-header", "-column", dbPath, query})
}

func (c *Client) execute(ctx context.Context, dbPath, query string) (string, error) {
	return c.run(ctx, []string{dbPath, query})
}

func (c *Client) tables(ctx context.Context, dbPath string) (string, error) {
	return c.run(ctx, []string{dbPath, ".tables"})
}

func (c *Client) schema(ctx context.Context, dbPath, table string) (string, error) {
	return c.run(ctx, []string{dbPath, fmt.Sprintf(".schema %s", table)})
}

func (c *Client) dump(ctx context.Context, dbPath string) (string, error) {
	return c.run(ctx, []string{dbPath, ".dump"})
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.sqlitePath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.sqlitePath, "--version")
	return cmd.Run() == nil
}
