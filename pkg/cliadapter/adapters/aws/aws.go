package aws

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	awsPath string
	region  string
	profile string
}

type Config struct {
	AWSPath string
	Region  string
	Profile string
}

func NewClient(cfg Config) *Client {
	path := cfg.AWSPath
	if path == "" {
		path = "aws"
	}
	return &Client{
		awsPath: path,
		region:  cfg.Region,
		profile: cfg.Profile,
	}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"sts", "get-caller-identity"})
	}

	service := args[0]
	switch service {
	case "s3":
		return c.s3(ctx, args[1:])
	case "ec2":
		return c.ec2(ctx, args[1:])
	case "lambda":
		return c.lambda(ctx, args[1:])
	case "rds":
		return c.rds(ctx, args[1:])
	case "iam":
		return c.iam(ctx, args[1:])
	case "ecs":
		return c.ecs(ctx, args[1:])
	case "eks":
		return c.eks(ctx, args[1:])
	case "dynamodb":
		return c.dynamodb(ctx, args[1:])
	case "logs":
		return c.logs(ctx, args[1:])
	case "cloudwatch":
		return c.cloudwatch(ctx, args[1:])
	case "ssm":
		return c.ssm(ctx, args[1:])
	case "configure":
		return c.configure(ctx, args[1:])
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) s3(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"s3", "ls"})
	}

	subcmd := args[0]
	switch subcmd {
	case "ls", "list":
		return c.run(ctx, []string{"s3", "ls", "s3://"})
	case "mb":
		return c.run(ctx, append([]string{"s3", "mb"}, args[1:]...))
	case "rb":
		return c.run(ctx, append([]string{"s3", "rb"}, args[1:]...))
	case "cp":
		return c.run(ctx, append([]string{"s3", "cp"}, args[1:]...))
	case "sync":
		return c.run(ctx, append([]string{"s3", "sync"}, args[1:]...))
	case "rm":
		return c.run(ctx, append([]string{"s3", "rm"}, args[1:]...))
	default:
		return c.run(ctx, append([]string{"s3", subcmd}, args[1:]...))
	}
}

func (c *Client) ec2(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"ec2", "describe-instances"})
	}

	return c.run(ctx, append([]string{"ec2"}, args...))
}

func (c *Client) lambda(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"lambda", "list-functions"})
	}

	return c.run(ctx, append([]string{"lambda"}, args...))
}

func (c *Client) rds(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"rds", "describe-db-instances"})
	}

	return c.run(ctx, append([]string{"rds"}, args...))
}

func (c *Client) iam(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"iam", "list-users"})
	}

	return c.run(ctx, append([]string{"iam"}, args...))
}

func (c *Client) ecs(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"ecs", "list-clusters"})
	}

	return c.run(ctx, append([]string{"ecs"}, args...))
}

func (c *Client) eks(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"eks", "list-clusters"})
	}

	return c.run(ctx, append([]string{"eks"}, args...))
}

func (c *Client) dynamodb(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"dynamodb", "list-tables"})
	}

	return c.run(ctx, append([]string{"dynamodb"}, args...))
}

func (c *Client) logs(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"logs", "describe-log-groups"})
	}

	return c.run(ctx, append([]string{"logs"}, args...))
}

func (c *Client) cloudwatch(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"cloudwatch", "list-metrics"})
	}

	return c.run(ctx, append([]string{"cloudwatch"}, args...))
}

func (c *Client) ssm(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.run(ctx, []string{"ssm", "list-commands"})
	}

	return c.run(ctx, append([]string{"ssm"}, args...))
}

func (c *Client) configure(ctx context.Context, args []string) (string, error) {
	configArgs := []string{"configure"}
	configArgs = append(configArgs, args...)

	if c.region != "" {
		fmt.Println("Region:", c.region)
	}
	if c.profile != "" {
		fmt.Println("Profile:", c.profile)
	}

	return c.run(ctx, configArgs)
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.awsPath, args...)
	if c.region != "" {
		cmd.Env = append(cmd.Env, "AWS_DEFAULT_REGION="+c.region)
	}
	if c.profile != "" {
		cmd.Env = append(cmd.Env, "AWS_PROFILE="+c.profile)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.awsPath, "--version")
	return cmd.Run() == nil
}
