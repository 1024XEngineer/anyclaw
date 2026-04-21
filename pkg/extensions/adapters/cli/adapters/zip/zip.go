package zip

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Client struct{}

type Config struct{}

func NewClient(cfg Config) *Client {
	return &Client{}
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: zip <command> [args]\nCommands: create, extract, list, add", nil
	}

	switch args[0] {
	case "create", "c":
		return c.create(ctx, args[1:])
	case "extract", "unzip", "x":
		return c.extract(ctx, args[1:])
	case "list", "l":
		return c.list(ctx, args[1:])
	case "add", "a":
		return c.add(ctx, args[1:])
	default:
		return "", fmt.Errorf("unknown command: %s", args[0])
	}
}

func (c *Client) create(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: zip create <archive> <files...>")
	}

	archive := args[0]
	files := args[1:]

	output, err := os.Create(archive)
	if err != nil {
		return "", err
	}
	defer output.Close()

	writer := zip.NewWriter(output)
	defer writer.Close()

	for _, file := range files {
		if err := c.addFile(writer, file); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("Created: %s with %d files", archive, len(files)), nil
}

func (c *Client) addFile(writer *zip.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = filepath.Base(path)

	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(entry, file)
	return err
}

func (c *Client) extract(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: zip extract <archive> [destination]")
	}

	archive := args[0]
	dest := "."
	if len(args) > 1 {
		dest = args[1]
	}

	reader, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}

	count := 0
	for _, file := range reader.File {
		path := filepath.Join(dest, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}

		out, err := os.Create(path)
		if err != nil {
			return "", err
		}

		rc, err := file.Open()
		if err != nil {
			out.Close()
			return "", err
		}

		io.Copy(out, rc)
		rc.Close()
		out.Close()
		count++
	}

	return fmt.Sprintf("Extracted %d files to %s", count, dest), nil
}

func (c *Client) list(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: zip list <archive>")
	}

	archive := args[0]

	reader, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var result []string
	result = append(result, fmt.Sprintf("Archive: %s", archive))
	result = append(result, fmt.Sprintf("Files: %d\n", len(reader.File)))

	for _, file := range reader.File {
		info := file.FileInfo()
		size := info.Size()
		if size == 0 {
			result = append(result, fmt.Sprintf("  %s/", file.Name))
		} else {
			result = append(result, fmt.Sprintf("  %s (%d bytes)", file.Name, size))
		}
	}

	return strings.Join(result, "\n"), nil
}

func (c *Client) add(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: zip add <archive> <files...>")
	}

	archive := args[0]
	files := args[1:]

	tempFile := archive + ".tmp"
	temp, err := os.Create(tempFile)
	if err != nil {
		return "", err
	}

	original, err := os.Open(archive)
	if err == nil {
		zipReader, _ := zip.OpenReader(archive)
		if zipReader != nil {
			writer := zip.NewWriter(temp)

			for _, file := range zipReader.File {
				rc, _ := file.Open()
				if rc != nil {
					header, _ := zip.FileInfoHeader(file.FileInfo())
					header.Name = file.Name
					entry, _ := writer.CreateHeader(header)
					io.Copy(entry, rc)
					rc.Close()
				}
			}

			for _, file := range files {
				c.addFile(writer, file)
			}

			writer.Close()
			original.Close()
			zipReader.Close()
		}
	}
	temp.Close()

	os.Rename(tempFile, archive)

	return fmt.Sprintf("Added %d files to %s", len(files), archive), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	return true
}

func ListZIP(archive string) ([]string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	files := make([]string, len(reader.File))
	for i, file := range reader.File {
		files[i] = file.Name
	}
	return files, nil
}
