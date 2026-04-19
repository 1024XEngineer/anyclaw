package python

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	pythonPath string
	venvPath   string
}

type Config struct {
	PythonPath string
	VenvPath   string
}

func NewClient(cfg Config) *Client {
	path := cfg.PythonPath
	if path == "" {
		path = "python"
	}
	return &Client{
		pythonPath: path,
		venvPath:   cfg.VenvPath,
	}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.version(ctx)
	}

	switch args[0] {
	case "run":
		return c.runScript(ctx, args[1:])
	case "install":
		return c.pipInstall(ctx, args[1:])
	case "pip":
		return c.pip(ctx, args[1:])
	case "version", "V":
		return c.version(ctx)
	case "info":
		return c.info(ctx)
	case "list":
		return c.listPackages(ctx)
	case "env":
		return c.venv(ctx)
	case "script":
		return c.runScriptFile(ctx, args[1:])
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) version(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"--version"})
}

func (c *Client) info(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"-c", "import sys; print(sys.executable); print(sys.version)"})
}

func (c *Client) runScript(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("code required")
	}

	code := strings.Join(args, " ")
	return c.run(ctx, []string{"-c", code})
}

func (c *Client) runScriptFile(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("script path required")
	}

	return c.run(ctx, args)
}

func (c *Client) pipInstall(ctx context.Context, args []string) (string, error) {
	pipArgs := []string{"-m", "pip", "install"}
	pipArgs = append(pipArgs, args...)

	_, err := c.run(ctx, pipArgs)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Installed: %s", strings.Join(args, " ")), nil
}

func (c *Client) pip(ctx context.Context, args []string) (string, error) {
	pipArgs := []string{"-m", "pip"}
	pipArgs = append(pipArgs, args...)

	return c.run(ctx, pipArgs)
}

func (c *Client) listPackages(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"-m", "pip", "list"})
}

func (c *Client) venv(ctx context.Context) (string, error) {
	if c.venvPath == "" {
		return c.run(ctx, []string{"-m", "venv"})
	}
	return c.run(ctx, []string{"-m", "venv", c.venvPath})
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.pythonPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.pythonPath, "--version")
	return cmd.Run() == nil
}
