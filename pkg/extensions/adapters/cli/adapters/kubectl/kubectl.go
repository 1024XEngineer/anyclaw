package kubectl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	kubectlPath string
	namespace   string
	kubeconfig  string
}

type Config struct {
	KubectlPath string
	Namespace   string
	Kubeconfig  string
}

func NewClient(cfg Config) *Client {
	path := cfg.KubectlPath
	if path == "" {
		path = "kubectl"
	}
	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}
	return &Client{
		kubectlPath: path,
		namespace:   ns,
		kubeconfig:  cfg.Kubeconfig,
	}
}

type Resource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Type      string `json:"type"`
	Status    string `json:"status,omitempty"`
	Age       string `json:"age"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.getPods(ctx)
	}

	switch args[0] {
	case "get", "g":
		return c.get(ctx, args[1:])
	case "describe", "desc":
		return c.describe(ctx, args[1:])
	case "apply", "a":
		return c.apply(ctx, args[1:])
	case "delete", "d":
		return c.delete(ctx, args[1:])
	case "create":
		return c.create(ctx, args[1:])
	case "logs", "log":
		return c.logs(ctx, args[1:])
	case "exec":
		return c.exec(ctx, args[1:])
	case "port-forward", "pf":
		return c.portForward(ctx, args[1:])
	case "scale":
		return c.scale(ctx, args[1:])
	case "rollout":
		return c.rollout(ctx, args[1:])
	case "top":
		return c.top(ctx, args[1:])
	case "cluster-info":
		return c.clusterInfo(ctx)
	case "config":
		return c.config(ctx, args[1:])
	case "api-resources":
		return c.apiResources(ctx)
	case "version":
		return c.version(ctx)
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) get(ctx context.Context, args []string) (string, error) {
	resourceType := "pods"
	if len(args) > 0 {
		if !isKnownResource(args[0]) {
			resourceType = args[0]
			args = args[1:]
		}
	}

	getArgs := []string{"get", resourceType}
	if c.namespace != "" {
		getArgs = append(getArgs, "-n", c.namespace)
	}
	getArgs = append(getArgs, "-o", "wide")
	getArgs = append(getArgs, args...)

	return c.run(ctx, getArgs)
}

func (c *Client) getPods(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"get", "pods", "-n", c.namespace, "-o", "wide"})
}

func (c *Client) describe(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("resource type and name required")
	}

	resourceType := args[0]
	name := ""
	if len(args) > 1 {
		name = args[1]
	}

	descArgs := []string{"describe", resourceType}
	if name != "" {
		descArgs = append(descArgs, name)
	}
	if c.namespace != "" {
		descArgs = append(descArgs, "-n", c.namespace)
	}

	return c.run(ctx, descArgs)
}

func (c *Client) apply(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("file or directory required")
	}

	applyArgs := []string{"apply", "-f"}
	applyArgs = append(applyArgs, args...)

	_, err := c.run(ctx, applyArgs)
	if err != nil {
		return "", err
	}

	return "Applied successfully", nil
}

func (c *Client) delete(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("resource required")
	}

	deleteArgs := []string{"delete"}
	if isKnownResource(args[0]) {
		resourceType := args[0]
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		deleteArgs = append(deleteArgs, resourceType)
		if name != "" {
			deleteArgs = append(deleteArgs, name)
		}
	} else {
		deleteArgs = append(deleteArgs, "-f", args[0])
	}

	deleteArgs = append(deleteArgs, "--cascade=foreground")

	_, err := c.run(ctx, deleteArgs)
	if err != nil {
		return "", err
	}

	return "Deleted successfully", nil
}

func (c *Client) create(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("file required")
	}

	createArgs := []string{"create", "-f"}
	createArgs = append(createArgs, args...)

	_, err := c.run(ctx, createArgs)
	if err != nil {
		return "", err
	}

	return "Created successfully", nil
}

func (c *Client) logs(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("pod name required")
	}

	logArgs := []string{"logs"}
	if c.namespace != "" {
		logArgs = append(logArgs, "-n", c.namespace)
	}
	logArgs = append(logArgs, args...)

	return c.run(ctx, logArgs)
}

func (c *Client) exec(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("pod name and command required")
	}

	pod := args[0]
	cmd := args[1:]

	execArgs := []string{"exec", "-n", c.namespace, pod, "--"}
	execArgs = append(execArgs, cmd...)

	return c.run(ctx, execArgs)
}

func (c *Client) portForward(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("pod and ports required")
	}

	pod := args[0]
	ports := args[1]

	return fmt.Sprintf("Port forward: kubectl port-forward %s %s -n %s", pod, ports, c.namespace), nil
}

func (c *Client) scale(ctx context.Context, args []string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("resource type, name and replica count required")
	}

	resourceType := args[0]
	name := args[1]
	replicas := args[2]

	scaleArgs := []string{"scale", resourceType, name, "--replicas=" + replicas}
	if c.namespace != "" {
		scaleArgs = append(scaleArgs, "-n", c.namespace)
	}

	_, err := c.run(ctx, scaleArgs)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Scaled %s/%s to %s replicas", resourceType, name, replicas), nil
}

func (c *Client) rollout(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("resource type and name required")
	}

	subcmd := args[0]
	resource := args[1]

	rolloutArgs := []string{"rollout", subcmd, resource}
	if c.namespace != "" {
		rolloutArgs = append(rolloutArgs, "-n", c.namespace)
	}
	if len(args) > 2 {
		rolloutArgs = append(rolloutArgs, args[2:]...)
	}

	return c.run(ctx, rolloutArgs)
}

func (c *Client) top(ctx context.Context, args []string) (string, error) {
	topArgs := []string{"top"}
	if c.namespace != "" {
		topArgs = append(topArgs, "-n", c.namespace)
	}
	topArgs = append(topArgs, args...)

	return c.run(ctx, topArgs)
}

func (c *Client) clusterInfo(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"cluster-info"})
}

func (c *Client) config(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"config", "current-context"})
	}

	configArgs := []string{"config"}
	configArgs = append(configArgs, args...)

	return c.run(ctx, configArgs)
}

func (c *Client) apiResources(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"api-resources", "-o", "wide"})
}

func (c *Client) version(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"version", "--client"})
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.kubectlPath, args...)
	if c.kubeconfig != "" {
		cmd.Env = append(cmd.Env, "KUBECONFIG="+c.kubeconfig)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.kubectlPath, "version", "--client")
	return cmd.Run() == nil
}

func isKnownResource(s string) bool {
	resources := []string{
		"pods", "po", "services", "svc", "deployments", "deploy",
		"replicasets", "rs", "statefulsets", "sts", "daemonsets", "ds",
		"configmaps", "cm", "secrets", "ingress", "ing", "persistentvolumeclaims", "pvc",
		"namespaces", "ns", "nodes", "no", "persistentvolumes", "pv",
		"jobs", "cronjobs", "cj", "poddisruptionbudgets", "pdb",
	}
	for _, r := range resources {
		if s == r {
			return true
		}
	}
	return false
}
