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

	writer := zip.NewWriter(output)

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			_ = output.Close()
			return "", err
		}
		if err := c.addFile(writer, file); err != nil {
			_ = writer.Close()
			_ = output.Close()
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		_ = output.Close()
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
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
		if err := ctx.Err(); err != nil {
			return "", err
		}
		path, err := safeExtractPath(dest, file.Name)
		if err != nil {
			return "", err
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return "", err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}

		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			return "", err
		}

		rc, err := file.Open()
		if err != nil {
			_ = out.Close()
			return "", err
		}

		_, copyErr := io.Copy(out, rc)
		closeReadErr := rc.Close()
		closeWriteErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeReadErr != nil {
			return "", closeReadErr
		}
		if closeWriteErr != nil {
			return "", closeWriteErr
		}
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
	defer os.Remove(tempFile)

	writer := zip.NewWriter(temp)
	zipReader, err := zip.OpenReader(archive)
	if err == nil {
		for _, file := range zipReader.File {
			if err := ctx.Err(); err != nil {
				_ = zipReader.Close()
				_ = writer.Close()
				_ = temp.Close()
				return "", err
			}
			if err := copyZipEntry(writer, file); err != nil {
				_ = zipReader.Close()
				_ = writer.Close()
				_ = temp.Close()
				return "", err
			}
		}
		if err := zipReader.Close(); err != nil {
			_ = writer.Close()
			_ = temp.Close()
			return "", err
		}
	} else if !os.IsNotExist(err) {
		_ = writer.Close()
		_ = temp.Close()
		return "", err
	}

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			_ = temp.Close()
			return "", err
		}
		if err := c.addFile(writer, file); err != nil {
			_ = writer.Close()
			_ = temp.Close()
			return "", err
		}
	}

	if err := writer.Close(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempFile, archive); err != nil {
		return "", err
	}

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

func safeExtractPath(dest, name string) (string, error) {
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("unsafe zip entry path: %s", name)
	}
	cleanName := filepath.Clean(name)
	if cleanName == "." || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." {
		return "", fmt.Errorf("unsafe zip entry path: %s", name)
	}

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	target := filepath.Join(destAbs, cleanName)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(destAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("unsafe zip entry path: %s", name)
	}
	if err := rejectSymlinkPath(destAbs, targetAbs, name); err != nil {
		return "", err
	}
	return targetAbs, nil
}

func rejectSymlinkPath(destAbs, targetAbs, name string) error {
	destAbs = filepath.Clean(destAbs)
	targetAbs = filepath.Clean(targetAbs)

	if info, err := os.Lstat(destAbs); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe zip entry path: %s", name)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	rel, err := filepath.Rel(destAbs, targetAbs)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	current := destAbs
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe zip entry path: %s", name)
		}
	}
	return nil
}

func copyZipEntry(writer *zip.Writer, file *zip.File) error {
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	header, err := zip.FileInfoHeader(file.FileInfo())
	if err != nil {
		return err
	}
	header.Name = file.Name
	header.Method = file.Method

	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, rc)
	return err
}
