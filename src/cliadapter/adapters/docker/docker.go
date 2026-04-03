package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	dockerPath string
}

type Config struct {
	DockerPath string
}

func NewClient(cfg Config) *Client {
	path := cfg.DockerPath
	if path == "" {
		path = "docker"
	}
	return &Client{dockerPath: path}
}

type Container struct {
	ID     string `json:"id"`
	Image  string `json:"image"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
	Names  string `json:"names"`
}

type Image struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	ID         string `json:"id"`
}

type Volume struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Mount  string `json:"mountpoint"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.ps(ctx)
	}

	cmd := args[0]
	switch cmd {
	case "ps", "list":
		return c.ps(ctx)
	case "images", "list-images":
		return c.images(ctx)
	case "run", "start":
		return c.run(ctx, args[1:])
	case "stop", "kill":
		return c.stop(ctx, args[1:])
	case "rm", "remove":
		return c.rm(ctx, args[1:])
	case "logs":
		return c.logs(ctx, args[1:])
	case "exec":
		return c.exec(ctx, args[1:])
	case "pull":
		return c.pull(ctx, args[1:])
	case "volumes", "vol":
		return c.volumes(ctx)
	case "info", "system":
		return c.info(ctx)
	case "search":
		return c.search(ctx, args[1:])
	default:
		return c.runDocker(ctx, args)
	}
}

func (c *Client) ps(ctx context.Context) (string, error) {
	out, err := c.runDocker(ctx, []string{"ps", "--format", "{{json .}}"})
	if err != nil {
		return "", err
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var ctr Container
		json.Unmarshal([]byte(line), &ctr)
		containers = append(containers, ctr)
	}

	if len(containers) == 0 {
		return "No running containers", nil
	}

	var result []string
	result = append(result, "CONTAINERS:")
	for _, ctr := range containers {
		result = append(result, fmt.Sprintf("  %s  %s  %s  %s", ctr.Names, ctr.ID[:12], ctr.Image, ctr.Status))
	}
	return strings.Join(result, "\n"), nil
}

func (c *Client) images(ctx context.Context) (string, error) {
	out, err := c.runDocker(ctx, []string{"images", "--format", "{{json .}}"})
	if err != nil {
		return "", err
	}

	var images []Image
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var img Image
		json.Unmarshal([]byte(line), &img)
		images = append(images, img)
	}

	if len(images) == 0 {
		return "No images", nil
	}

	var result []string
	result = append(result, "IMAGES:")
	for _, img := range images {
		result = append(result, fmt.Sprintf("  %s:%s  %s", img.Repository, img.Tag, img.Size))
	}
	return strings.Join(result, "\n"), nil
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	containerName := ""
	image := ""
	cmdArgs := []string{}

	for i, arg := range args {
		if arg == "-d" || arg == "-it" || arg == "--detach" {
			continue
		}
		if arg == "--name" && i+1 < len(args) {
			containerName = args[i+1]
			continue
		}
		if !strings.HasPrefix(arg, "-") && image == "" {
			image = arg
			continue
		}
		cmdArgs = append(cmdArgs, arg)
	}

	if image == "" {
		return "", fmt.Errorf("image required")
	}

	dockerArgs := []string{"run", "-d"}
	if containerName != "" {
		dockerArgs = append(dockerArgs, "--name", containerName)
	}
	dockerArgs = append(dockerArgs, image)
	dockerArgs = append(dockerArgs, cmdArgs...)

	_, err := c.runDocker(ctx, dockerArgs)
	if err != nil {
		return "", err
	}

	if containerName != "" {
		return fmt.Sprintf("Container %s started", containerName), nil
	}
	return fmt.Sprintf("Container started from image %s", image), nil
}

func (c *Client) stop(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("container name or ID required")
	}

	_, err := c.runDocker(ctx, []string{"stop", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Container %s stopped", args[0]), nil
}

func (c *Client) rm(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("container name or ID required")
	}

	_, err := c.runDocker(ctx, []string{"rm", "-f", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Container %s removed", args[0]), nil
}

func (c *Client) logs(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("container name or ID required")
	}

	dockerArgs := []string{"logs"}
	if len(args) > 1 && args[1] == "--follow" {
		dockerArgs = append(dockerArgs, "-f")
		args = args[1:]
	}
	dockerArgs = append(dockerArgs, args[0])

	if len(args) > 1 {
		dockerArgs = append(dockerArgs, args[1:]...)
	}

	return c.runDocker(ctx, dockerArgs)
}

func (c *Client) exec(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: docker exec <container> <command>")
	}

	container := args[0]
	cmd := strings.Join(args[1:], " ")

	return c.runDocker(ctx, []string{"exec", container, "sh", "-c", cmd})
}

func (c *Client) pull(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("image name required")
	}

	_, err := c.runDocker(ctx, []string{"pull", args[0]})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Image %s pulled", args[0]), nil
}

func (c *Client) volumes(ctx context.Context) (string, error) {
	out, err := c.runDocker(ctx, []string{"volume", "ls", "--format", "{{json .}}"})
	if err != nil {
		return "", err
	}

	var volumes []Volume
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var vol Volume
		json.Unmarshal([]byte(line), &vol)
		volumes = append(volumes, vol)
	}

	if len(volumes) == 0 {
		return "No volumes", nil
	}

	var result []string
	result = append(result, "VOLUMES:")
	for _, vol := range volumes {
		result = append(result, fmt.Sprintf("  %s", vol.Name))
	}
	return strings.Join(result, "\n"), nil
}

func (c *Client) info(ctx context.Context) (string, error) {
	return c.runDocker(ctx, []string{"info", "--format", "{{.ServerVersion}}"})
}

func (c *Client) search(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("search term required")
	}

	out, err := c.runDocker(ctx, []string{"search", "--limit", "10", args[0]})
	if err != nil {
		return "", err
	}

	return out, nil
}

func (c *Client) runDocker(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.dockerPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.dockerPath, "version")
	return cmd.Run() == nil
}
