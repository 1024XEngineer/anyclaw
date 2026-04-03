package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	gitPath string
	repoDir string
}

type Config struct {
	GitPath string
	RepoDir string
}

func NewClient(cfg Config) *Client {
	gitPath := cfg.GitPath
	if gitPath == "" {
		gitPath = "git"
	}
	repoDir := cfg.RepoDir
	if repoDir == "" {
		repoDir = "."
	}
	return &Client{
		gitPath: gitPath,
		repoDir: repoDir,
	}
}

type Status struct {
	Branch    string   `json:"branch"`
	Staged    []string `json:"staged"`
	Modified  []string `json:"modified"`
	Untracked []string `json:"untracked"`
	Conflict  []string `json:"conflict"`
}

type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

type Branch struct {
	Name   string `json:"name"`
	Remote bool   `json:"remote"`
	Head   bool   `json:"head"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.status(ctx)
	}

	switch args[0] {
	case "status", "st":
		return c.status(ctx)
	case "log":
		return c.log(ctx, args[1:])
	case "branch", "br":
		return c.branch(ctx, args[1:])
	case "commit", "ci":
		return c.commit(ctx, args[1:])
	case "add":
		return c.add(ctx, args[1:])
	case "push":
		return c.push(ctx, args[1:])
	case "pull":
		return c.pull(ctx, args[1:])
	case "clone":
		return c.clone(ctx, args[1:])
	case "checkout", "co":
		return c.checkout(ctx, args[1:])
	case "diff":
		return c.diff(ctx, args[1:])
	case "merge":
		return c.merge(ctx, args[1:])
	case "rebase":
		return c.rebase(ctx, args[1:])
	case "stash":
		return c.stash(ctx, args[1:])
	case "remote", "remote -v":
		return c.remote(ctx)
	case "fetch":
		return c.fetch(ctx, args[1:])
	case "init":
		return c.init(ctx, args[1:])
	case "tag":
		return c.tag(ctx, args[1:])
	case "show":
		return c.show(ctx, args[1:])
	case "reset":
		return c.reset(ctx, args[1:])
	case "clean":
		return c.clean(ctx, args[1:])
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) status(ctx context.Context) (string, error) {
	output, err := c.run(ctx, []string{"status", "--porcelain"})
	if err != nil {
		return "", err
	}

	status := &Status{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		file := strings.TrimSpace(line[3:])
		switch line[:2] {
		case "M ":
			status.Modified = append(status.Modified, file)
		case "A ":
			status.Staged = append(status.Staged, file)
		case "D ":
			status.Modified = append(status.Modified, file)
		case "??":
			status.Untracked = append(status.Untracked, file)
		case "UU":
			status.Conflict = append(status.Conflict, file)
		}
	}

	branch, _ := c.run(ctx, []string{"branch", "--show-current"})
	status.Branch = strings.TrimSpace(branch)

	var result []string
	result = append(result, fmt.Sprintf("On branch: %s", status.Branch))
	if len(status.Staged) > 0 {
		result = append(result, "Staged:")
		for _, f := range status.Staged {
			result = append(result, fmt.Sprintf("  %s", f))
		}
	}
	if len(status.Modified) > 0 {
		result = append(result, "Modified:")
		for _, f := range status.Modified {
			result = append(result, fmt.Sprintf("  %s", f))
		}
	}
	if len(status.Untracked) > 0 {
		result = append(result, "Untracked:")
		for _, f := range status.Untracked {
			result = append(result, fmt.Sprintf("  %s", f))
		}
	}

	return strings.Join(result, "\n"), nil
}

func (c *Client) log(ctx context.Context, args []string) (string, error) {
	limit := "10"
	for i, arg := range args {
		if arg == "-n" && i+1 < len(args) {
			limit = args[i+1]
		}
	}

	output, err := c.run(ctx, []string{"log", "--oneline", "-n", limit})
	return output, err
}

func (c *Client) branch(ctx context.Context, args []string) (string, error) {
	output, err := c.run(ctx, []string{"branch", "-a"})
	if err != nil {
		return "", err
	}

	var result []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "* ") {
			result = append(result, fmt.Sprintf("* %s (current)", strings.TrimPrefix(line, "* ")))
		} else {
			result = append(result, "  "+line)
		}
	}

	return strings.Join(result, "\n"), nil
}

func (c *Client) commit(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("commit message required")
	}

	msg := strings.Join(args, " ")
	_, err := c.run(ctx, []string{"commit", "-m", msg})
	if err != nil {
		return "", err
	}

	return "Committed: " + msg, nil
}

func (c *Client) add(ctx context.Context, args []string) (string, error) {
	files := []string{"."}
	if len(args) > 0 {
		files = args
	}

	_, err := c.run(ctx, append([]string{"add"}, files...))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Added %d files", len(files)), nil
}

func (c *Client) push(ctx context.Context, args []string) (string, error) {
	pushArgs := []string{"push"}
	pushArgs = append(pushArgs, args...)

	_, err := c.run(ctx, pushArgs)
	if err != nil {
		return "", err
	}

	return "Pushed successfully", nil
}

func (c *Client) pull(ctx context.Context, args []string) (string, error) {
	pullArgs := []string{"pull"}
	pullArgs = append(pullArgs, args...)

	_, err := c.run(ctx, pullArgs)
	if err != nil {
		return "", err
	}

	return "Pulled successfully", nil
}

func (c *Client) clone(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("repository URL required")
	}

	_, err := c.run(ctx, []string{"clone", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Cloned: %s", args[0]), nil
}

func (c *Client) checkout(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("branch name required")
	}

	_, err := c.run(ctx, []string{"checkout", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Switched to: %s", args[0]), nil
}

func (c *Client) diff(ctx context.Context, args []string) (string, error) {
	diffArgs := []string{"diff"}
	diffArgs = append(diffArgs, args...)

	return c.run(ctx, diffArgs)
}

func (c *Client) merge(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("branch name required")
	}

	_, err := c.run(ctx, []string{"merge", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Merged: %s", args[0]), nil
}

func (c *Client) rebase(ctx context.Context, args []string) (string, error) {
	branch := "main"
	if len(args) > 0 {
		branch = args[0]
	}

	_, err := c.run(ctx, []string{"rebase", branch})
	if err != nil {
		return "", err
	}

	return "Rebased successfully", nil
}

func (c *Client) stash(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		output, _ := c.run(ctx, []string{"stash", "list"})
		return output, nil
	}

	switch args[0] {
	case "push", "save":
		msg := "WIP"
		if len(args) > 1 {
			msg = args[1]
		}
		_, err := c.run(ctx, []string{"stash", "push", "-m", msg})
		if err != nil {
			return "", err
		}
		return "Stashed", nil
	case "pop":
		_, err := c.run(ctx, []string{"stash", "pop"})
		return "Stash applied", err
	case "list":
		return c.run(ctx, []string{"stash", "list"})
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) remote(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"remote", "-v"})
}

func (c *Client) fetch(ctx context.Context, args []string) (string, error) {
	fetchArgs := []string{"fetch"}
	fetchArgs = append(fetchArgs, args...)

	_, err := c.run(ctx, fetchArgs)
	if err != nil {
		return "", err
	}

	return "Fetched successfully", nil
}

func (c *Client) init(ctx context.Context, args []string) (string, error) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	_, err := c.run(ctx, []string{"init", dir})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Initialized: %s", dir), nil
}

func (c *Client) tag(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"tag", "-l"})
	}

	name := args[0]
	msg := ""
	if len(args) > 1 {
		msg = args[1]
	}

	if msg != "" {
		_, err := c.run(ctx, []string{"tag", "-a", name, "-m", msg})
		return fmt.Sprintf("Tag created: %s", name), err
	}

	_, err := c.run(ctx, []string{"tag", name})
	return fmt.Sprintf("Tag created: %s", name), err
}

func (c *Client) show(ctx context.Context, args []string) (string, error) {
	rev := "HEAD"
	if len(args) > 0 {
		rev = args[0]
	}

	return c.run(ctx, []string{"show", rev})
}

func (c *Client) reset(ctx context.Context, args []string) (string, error) {
	mode := "--soft"
	if len(args) > 0 && strings.HasPrefix(args[0], "--") {
		mode = args[0]
		args = args[1:]
	}

	commit := "HEAD~1"
	if len(args) > 0 {
		commit = args[0]
	}

	_, err := c.run(ctx, []string{"reset", mode, commit})
	if err != nil {
		return "", err
	}

	return "Reset successful", nil
}

func (c *Client) clean(ctx context.Context, args []string) (string, error) {
	cleanArgs := []string{"clean", "-fd"}
	cleanArgs = append(cleanArgs, args...)

	_, err := c.run(ctx, cleanArgs)
	if err != nil {
		return "", err
	}

	return "Cleaned untracked files", nil
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.gitPath, args...)
	cmd.Dir = c.repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.gitPath, "--version")
	return cmd.Run() == nil
}
