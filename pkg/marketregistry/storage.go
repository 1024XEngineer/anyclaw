package marketregistry

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type PackageStorage interface {
	EnsurePackage(artifact Artifact, version ArtifactVersion) (PackageInfo, error)
	Open(storageKey string) (*os.File, error)
}

type LocalStorage struct {
	packagesDir string
}

type PackageInfo struct {
	StorageKey     string
	Path           string
	SizeBytes      int64
	ChecksumSHA256 string
}

func NewLocalStorage(dataDir string) (*LocalStorage, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = ".anyclaw-registry"
	}
	storage := &LocalStorage{packagesDir: filepath.Join(dataDir, "packages")}
	if err := os.MkdirAll(storage.packagesDir, 0o755); err != nil {
		return nil, err
	}
	return storage, nil
}

func (s *LocalStorage) EnsurePackage(artifact Artifact, version ArtifactVersion) (PackageInfo, error) {
	if s == nil {
		return PackageInfo{}, fmt.Errorf("storage is not configured")
	}
	storageKey := filepath.ToSlash(filepath.Join(artifact.ID, version.Version, "artifact.zip"))
	path := filepath.Join(s.packagesDir, artifact.ID, version.Version, "artifact.zip")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return PackageInfo{}, err
	}
	if _, err := os.Stat(path); errorsIsNotExist(err) {
		if err := writePackage(path, artifact, version); err != nil {
			return PackageInfo{}, err
		}
	} else if err != nil {
		return PackageInfo{}, err
	}
	info, err := checksumFile(path)
	if err != nil {
		return PackageInfo{}, err
	}
	info.StorageKey = storageKey
	info.Path = path
	return info, nil
}

func (s *LocalStorage) Open(storageKey string) (*os.File, error) {
	if s == nil {
		return nil, fmt.Errorf("storage is not configured")
	}
	clean := filepath.Clean(filepath.FromSlash(storageKey))
	path := filepath.Join(s.packagesDir, clean)
	base, err := filepath.Abs(s.packagesDir)
	if err != nil {
		return nil, err
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if target != base && !strings.HasPrefix(target, base+string(os.PathSeparator)) {
		return nil, fmt.Errorf("invalid storage key")
	}
	return os.Open(target)
}

func writePackage(path string, artifact Artifact, version ArtifactVersion) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	manifest := artifact
	manifest.Version = version.Version
	manifest.SizeBytes = 0
	manifest.ChecksumSHA256 = ""
	if err := writeZipJSON(zw, "anyclaw.artifact.json", manifest); err != nil {
		return err
	}
	if err := writeZipText(zw, "README.md", fmt.Sprintf("# %s\n\n%s\n", artifact.Name, artifact.Summary)); err != nil {
		return err
	}

	switch artifact.Kind {
	case ArtifactKindAgent:
		return writeZipJSON(zw, "agent/profile.json", map[string]any{
			"id":          artifact.ID,
			"name":        artifact.Name,
			"description": artifact.Summary,
		})
	case ArtifactKindSkill:
		return writeZipText(zw, "skill/SKILL.md", fmt.Sprintf("# %s\n\n%s\n", artifact.Name, artifact.DescriptionMD))
	case ArtifactKindCLI:
		return writeZipJSON(zw, "cli/command.json", map[string]any{
			"id":      artifact.ID,
			"name":    artifact.Name,
			"command": artifact.ManifestSummary["command"],
		})
	default:
		return nil
	}
}

func writeZipJSON(zw *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeZipBytes(zw, name, data)
}

func writeZipText(zw *zip.Writer, name, text string) error {
	return writeZipBytes(zw, name, []byte(text))
}

func writeZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func checksumFile(path string) (PackageInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return PackageInfo{}, err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return PackageInfo{}, err
	}
	return PackageInfo{
		SizeBytes:      size,
		ChecksumSHA256: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
