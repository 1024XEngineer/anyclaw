package node

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	nodePath string
	npmPath  string
}

type Config struct {
	NodePath string
	NPMPath  string
}

func NewClient(cfg Config) *Client {
	nodePath := cfg.NodePath
	if nodePath == "" {
		nodePath = "node"
	}
	npmPath := cfg.NPMPath
	if npmPath == "" {
		npmPath = "npm"
	}
	return &Client{
		nodePath: nodePath,
		npmPath:  npmPath,
	}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.version(ctx)
	}

	switch args[0] {
	case "run", "r":
		return c.runScript(ctx, args[1:])
	case "exec", "e":
		return c.exec(ctx, args[1:])
	case "version", "v":
		return c.version(ctx)
	case "npm":
		return c.npm(ctx, args[1:])
	case "npx":
		return c.npx(ctx, args[1:])
	case "info":
		return c.info(ctx)
	case "REPL":
		return c.repl(ctx)
	default:
		if strings.HasSuffix(args[0], ".js") {
			return c.runFile(ctx, args)
		}
		return c.run(ctx, args)
	}
}

func (c *Client) version(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"--version"})
}

func (c *Client) info(ctx context.Context) (string, error) {
	nodeVersion, _ := c.run(ctx, []string{"--version"})
	npmVersion, _ := c.run(c.ctxWithNPM(ctx), []string{"--version"})
	return fmt.Sprintf("Node: %s\nNPM: %s", strings.TrimSpace(nodeVersion), strings.TrimSpace(npmVersion)), nil
}

func (c *Client) ctxWithNPM(ctx context.Context) context.Context {
	return ctx
}

func (c *Client) runScript(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(c.ctxWithNPM(ctx), []string{c.npmPath, "run"})
	}

	return c.run(c.ctxWithNPM(ctx), append([]string{c.npmPath, "run"}, args...))
}

func (c *Client) exec(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("code required")
	}

	code := strings.Join(args, " ")
	return c.run(ctx, []string{"-e", code})
}

func (c *Client) npm(ctx context.Context, args []string) (string, error) {
	npmArgs := []string{c.npmPath}
	npmArgs = append(npmArgs, args...)
	return c.run(ctx, npmArgs)
}

func (c *Client) npx(ctx context.Context, args []string) (string, error) {
	return c.run(ctx, append([]string{"npx"}, args...))
}

func (c *Client) runFile(ctx context.Context, args []string) (string, error) {
	return c.run(ctx, args)
}

func (c *Client) repl(ctx context.Context) (string, error) {
	return "Entering Node.js REPL. Type .exit to exit.", nil
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.nodePath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.nodePath, "--version")
	return cmd.Run() == nil
}
