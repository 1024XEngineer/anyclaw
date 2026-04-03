package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	npmPath    string
	workingDir string
}

type Config struct {
	NPMPath    string
	WorkingDir string
}

func NewClient(cfg Config) *Client {
	path := cfg.NPMPath
	if path == "" {
		path = "npm"
	}
	dir := cfg.WorkingDir
	if dir == "" {
		dir = "."
	}
	return &Client{
		npmPath:    path,
		workingDir: dir,
	}
}

type PackageInfo struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Dependencies map[string]string `json:"dependencies"`
}

type PackageList struct {
	Dependencies map[string]PackageInfo `json:"dependencies"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.list(ctx)
	}

	switch args[0] {
	case "install", "i":
		return c.install(ctx, args[1:])
	case "uninstall", "remove", "rm":
		return c.uninstall(ctx, args[1:])
	case "update", "up":
		return c.update(ctx, args[1:])
	case "list", "ls":
		return c.listArgs(ctx, args[1:])
	case "outdated":
		return c.outdated(ctx)
	case "init":
		return c.init(ctx, args[1:])
	case "run":
		return c.runScript(ctx, args[1:])
	case "start":
		return c.runScript(ctx, []string{"start"})
	case "test":
		return c.runScript(ctx, []string{"test"})
	case "publish":
		return c.publish(ctx, args[1:])
	case "info":
		return c.info(ctx, args[1:])
	case "search":
		return c.search(ctx, args[1:])
	case "version":
		return c.version(ctx, args[1:])
	case "view":
		return c.view(ctx, args[1:])
	case "doc", "docs":
		return c.docs(ctx, args[1:])
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) install(ctx context.Context, args []string) (string, error) {
	installArgs := []string{"install"}
	installArgs = append(installArgs, args...)

	_, err := c.run(ctx, installArgs)
	if err != nil {
		return "", err
	}

	if len(args) == 0 {
		return "Installed all dependencies from package.json", nil
	}
	return fmt.Sprintf("Installed: %s", strings.Join(args, " ")), nil
}

func (c *Client) uninstall(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("package name required")
	}

	_, err := c.run(ctx, []string{"uninstall", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Uninstalled: %s", args[0]), nil
}

func (c *Client) update(ctx context.Context, args []string) (string, error) {
	updateArgs := []string{"update"}
	if len(args) > 0 {
		updateArgs = append(updateArgs, args...)
	}

	_, err := c.run(ctx, updateArgs)
	if err != nil {
		return "", err
	}

	return "Updated packages", nil
}

func (c *Client) list(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"list", "--depth=0"})
}

func (c *Client) listArgs(ctx context.Context, args []string) (string, error) {
	listArgs := []string{"list"}
	listArgs = append(listArgs, args...)

	return c.run(ctx, listArgs)
}

func (c *Client) outdated(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"outdated"})
}

func (c *Client) init(ctx context.Context, args []string) (string, error) {
	initArgs := []string{"init"}
	if len(args) > 0 {
		initArgs = append(initArgs, "-y")
	}

	_, err := c.run(ctx, initArgs)
	if err != nil {
		return "", err
	}

	return "Initialized package.json", nil
}

func (c *Client) runScript(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"run"})
	}

	scriptArgs := []string{"run"}
	scriptArgs = append(scriptArgs, args...)

	return c.run(ctx, scriptArgs)
}

func (c *Client) publish(ctx context.Context, args []string) (string, error) {
	publishArgs := []string{"publish"}
	publishArgs = append(publishArgs, args...)

	_, err := c.run(ctx, publishArgs)
	if err != nil {
		return "", err
	}

	return "Published to npm", nil
}

func (c *Client) info(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("package name required")
	}

	return c.run(ctx, []string{"info", args[0]})
}

func (c *Client) search(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("search term required")
	}

	searchArgs := []string{"search", "--long"}
	searchArgs = append(searchArgs, args...)

	return c.run(ctx, searchArgs)
}

func (c *Client) version(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"--version"})
	}

	_, err := c.run(ctx, []string{"version", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Version updated to %s", args[0]), nil
}

func (c *Client) view(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("package name required")
	}

	viewArgs := []string{"view"}
	viewArgs = append(viewArgs, args...)

	return c.run(ctx, viewArgs)
}

func (c *Client) docs(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("package name required")
	}

	return fmt.Sprintf("https://www.npmjs.com/package/%s", args[0]), nil
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.npmPath, args...)
	cmd.Dir = c.workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.npmPath, "--version")
	return cmd.Run() == nil
}

func ParseDependencies(output string) (map[string]string, error) {
	var pkgList PackageList
	if err := json.Unmarshal([]byte(output), &pkgList); err != nil {
		return nil, err
	}

	deps := make(map[string]string)
	for name, info := range pkgList.Dependencies {
		deps[name] = info.Version
	}
	return deps, nil
}
