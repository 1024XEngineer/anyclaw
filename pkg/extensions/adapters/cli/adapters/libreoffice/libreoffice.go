package libreoffice

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	sofficePath string
}

type Config struct {
	SofficePath string
}

func NewClient(cfg Config) *Client {
	path := cfg.SofficePath
	if path == "" {
		path = "soffice"
	}
	return &Client{sofficePath: path}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "LibreOffice available. Use: convert, pdf, view", nil
	}

	switch args[0] {
	case "convert":
		return c.convert(ctx, args[1:])
	case "pdf":
		return c.toPDF(ctx, args[1:])
	case "view":
		return c.view(ctx, args[1:])
	case "headless":
		return c.headless(ctx, args[1:])
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) convert(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: libreoffice convert <input> <output>")
	}

	input := args[0]
	output := args[1]

	inputAbs, _ := filepath.Abs(input)
	outputAbs, _ := filepath.Abs(output)

	_, err := c.run(ctx, []string{
		"--headless",
		"--convert-to",
		filepath.Ext(output)[1:],
		"--outdir",
		filepath.Dir(outputAbs),
		inputAbs,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Converted: %s -> %s", input, output), nil
}

func (c *Client) toPDF(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: libreoffice pdf <input> [output]")
	}

	input := args[0]
	var output string
	if len(args) > 1 {
		output = args[1]
	} else {
		ext := filepath.Ext(input)
		output = strings.TrimSuffix(input, ext) + ".pdf"
	}

	inputAbs, _ := filepath.Abs(input)
	outputAbs, _ := filepath.Abs(output)

	_, err := c.run(ctx, []string{
		"--headless",
		"--convert-to",
		"pdf",
		"--outdir",
		filepath.Dir(outputAbs),
		inputAbs,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("PDF created: %s", output), nil
}

func (c *Client) view(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: libreoffice view <file>")
	}

	input := args[0]
	inputAbs, _ := filepath.Abs(input)

	_, err := c.run(ctx, []string{
		"--view",
		inputAbs,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Opened: %s", input), nil
}

func (c *Client) headless(ctx context.Context, args []string) (string, error) {
	headlessArgs := []string{"--headless"}
	headlessArgs = append(headlessArgs, args...)

	return c.run(ctx, headlessArgs)
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.sofficePath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.sofficePath, "--version")
	return cmd.Run() == nil
}
