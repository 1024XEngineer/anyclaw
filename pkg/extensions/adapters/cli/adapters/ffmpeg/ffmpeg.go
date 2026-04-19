package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Client struct {
	ffmpegPath  string
	ffprobePath string
}

type Config struct {
	FFmpegPath  string
	FFProbePath string
}

func NewClient(cfg Config) *Client {
	ffmpeg := cfg.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	ffprobe := cfg.FFProbePath
	if ffprobe == "" {
		ffprobe = "ffprobe"
	}
	return &Client{
		ffmpegPath:  ffmpeg,
		ffprobePath: ffprobe,
	}
}

type Stream struct {
	Index   int    `json:"index"`
	Type    string `json:"codec_type"`
	Codec   string `json:"codec_name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	FPS     string `json:"r_frame_rate"`
	Bitrate string `json:"bit_rate"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.probeMedia(ctx)
	}

	switch args[0] {
	case "info", "probe":
		return c.probeMediaArgs(ctx, args[1:])
	case "convert", "transcode":
		return c.convertMedia(ctx, args[1:])
	case "thumbnail", "screenshot":
		return c.thumbnailMedia(ctx, args[1:])
	case "version":
		return c.run(ctx, []string{"-version"})
	default:
		return c.run(ctx, args)
	}
}

func (c *Client) probeMedia(ctx context.Context) (string, error) {
	return c.run(ctx, []string{"-version"})
}

func (c *Client) probeMediaArgs(ctx context.Context, args []string) (string, error) {
	input := "input.mp4"
	if len(args) > 0 {
		input = args[0]
	}

	output, err := c.runFFProbe(ctx, []string{"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", input})
	if err != nil {
		return "", err
	}

	var result []string
	result = append(result, fmt.Sprintf("File: %s", input))
	result = append(result, output[:min(500, len(output))])
	return strings.Join(result, "\n"), nil
}

func (c *Client) convertMedia(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: ffmpeg convert <input> <output>")
	}

	input := args[0]
	output := args[1]

	_, err := c.run(ctx, []string{"-i", input, "-y", output})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Converted: %s -> %s", input, output), nil
}

func (c *Client) thumbnailMedia(ctx context.Context, args []string) (string, error) {
	input := "input.mp4"
	output := "thumb.jpg"
	time := "00:00:01"

	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) {
			input = args[i+1]
		}
		if arg == "-o" && i+1 < len(args) {
			output = args[i+1]
		}
		if arg == "-t" && i+1 < len(args) {
			time = args[i+1]
		}
	}

	_, err := c.run(ctx, []string{"-i", input, "-ss", time, "-vframes", "1", "-y", output})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Thumbnail: %s at %s", output, time), nil
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) runFFProbe(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.ffprobePath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.ffmpegPath, "-version")
	return cmd.Run() == nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getExt(filename string) string {
	ext := filepath.Ext(filename)
	return strings.ToLower(ext)
}

func audioCodec(ext string) string {
	switch ext {
	case ".mp3":
		return "libmp3lame"
	case ".wav":
		return "pcm_s16le"
	case ".aac", ".m4a":
		return "aac"
	default:
		return "copy"
	}
}
