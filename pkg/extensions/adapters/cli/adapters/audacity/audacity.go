package audacity

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	soxPath string
}

type Config struct {
	SoxPath string
}

func NewClient(cfg Config) *Client {
	soxPath := cfg.SoxPath
	if soxPath == "" {
		soxPath = "sox"
	}

	return &Client{
		soxPath: soxPath,
	}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.info(ctx)
	}

	switch args[0] {
	case "info", "version":
		return c.info(ctx)
	case "convert":
		return c.convert(ctx, args[1:])
	case "trim":
		return c.trim(ctx, args[1:])
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) info(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"--version"})
}

func (c *Client) convert(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: audacity convert <input> <output>")
	}

	input := args[0]
	output := args[1]
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(output)), ".")
	if format == "" {
		return "", fmt.Errorf("output file must include an extension")
	}

	_, err := c.run(ctx, []string{input, "-t", format, output})
	if err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}

	return fmt.Sprintf("Converted %s to %s", input, output), nil
}

func (c *Client) trim(ctx context.Context, args []string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("usage: audacity trim <input> <output> <duration>")
	}

	input := args[0]
	output := args[1]
	duration := args[2]

	_, err := c.run(ctx, []string{input, output, "trim", "0", duration})
	if err != nil {
		return "", fmt.Errorf("trim failed: %w", err)
	}

	return fmt.Sprintf("Trimmed %s to %s seconds", output, duration), nil
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.soxPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}

	return strings.TrimSpace(string(output)), nil
}
