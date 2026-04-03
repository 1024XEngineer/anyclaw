package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Type string

const (
	TypeImage Type = "image"
	TypeAudio Type = "audio"
	TypeVideo Type = "video"
	TypeDoc   Type = "document"
)

type Media struct {
	ID       string
	Type     Type
	Name     string
	Size     int64
	MimeType string
	Data     []byte
	Base64   string
	Path     string
	URL      string
	Metadata map[string]any
}

type Processor struct {
	mu           sync.RWMutex
	storagePath  string
	maxFileSize  int64
	allowedTypes []Type
	httpClient   *http.Client
}

func NewProcessor(storagePath string) *Processor {
	return &Processor{
		storagePath:  storagePath,
		maxFileSize:  10 * 1024 * 1024,
		allowedTypes: []Type{TypeImage, TypeAudio, TypeVideo, TypeDoc},
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Processor) Process(ctx context.Context, input *Media) (*Media, error) {
	switch input.Type {
	case TypeImage:
		return p.processImage(ctx, input)
	case TypeAudio:
		return p.processAudio(ctx, input)
	case TypeVideo:
		return p.processVideo(ctx, input)
	default:
		return input, nil
	}
}

func (p *Processor) processImage(ctx context.Context, input *Media) (*Media, error) {
	if len(input.Data) == 0 && input.Path != "" {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return nil, fmt.Errorf("read image file: %w", err)
		}
		input.Data = data
	}

	if len(input.Data) > 0 {
		img, _, err := image.Decode(bytes.NewReader(input.Data))
		if err != nil {
			return nil, fmt.Errorf("decode image: %w", err)
		}

		input.Metadata = map[string]any{
			"width":  img.Bounds().Dx(),
			"height": img.Bounds().Dy(),
		}
	}

	return input, nil
}

func (p *Processor) processAudio(ctx context.Context, input *Media) (*Media, error) {
	input.Metadata = map[string]any{
		"duration": 0,
		"format":   input.MimeType,
	}
	return input, nil
}

func (p *Processor) processVideo(ctx context.Context, input *Media) (*Media, error) {
	input.Metadata = map[string]any{
		"duration": 0,
		"format":   input.MimeType,
	}
	return input, nil
}

func (p *Processor) Download(ctx context.Context, url string) (*Media, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, p.maxFileSize))
	if err != nil {
		return nil, err
	}

	mediaType := resp.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	media := &Media{
		ID:       generateID(),
		Type:     p.guessType(mediaType),
		MimeType: mediaType,
		Size:     int64(len(data)),
		Data:     data,
		Base64:   base64.StdEncoding.EncodeToString(data),
		URL:      url,
	}

	return p.Process(ctx, media)
}

func (p *Processor) Upload(ctx context.Context, media *Media) (string, error) {
	if p.storagePath == "" {
		return "", nil
	}

	ext := extensionFromMime(media.MimeType)
	filename := fmt.Sprintf("%s%s", media.ID, ext)
	path := filepath.Join(p.storagePath, filename)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(path, media.Data, 0644); err != nil {
		return "", err
	}

	return path, nil
}

func (p *Processor) Save(data []byte, filename string) (string, error) {
	if p.storagePath == "" {
		return "", nil
	}

	path := filepath.Join(p.storagePath, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	return path, nil
}

func (p *Processor) Load(path string) (*Media, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return &Media{
		ID:       generateID(),
		Type:     p.guessType(mimeType),
		Name:     filepath.Base(path),
		Size:     info.Size(),
		MimeType: mimeType,
		Data:     data,
		Base64:   base64.StdEncoding.EncodeToString(data),
		Path:     path,
	}, nil
}

func (p *Processor) guessType(mimeType string) Type {
	if strings.HasPrefix(mimeType, "image/") {
		return TypeImage
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return TypeAudio
	}
	if strings.HasPrefix(mimeType, "video/") {
		return TypeVideo
	}
	return TypeDoc
}

func extensionFromMime(mimeType string) string {
	ext := mime.TypeByExtension(mimeType)
	if ext == "" {
		return ".bin"
	}
	return ext
}

func generateID() string {
	return fmt.Sprintf("media-%d", time.Now().UnixNano())
}

func FromMultipart(form *multipart.Form) ([]*Media, error) {
	var media []*Media

	for _, files := range form.File {
		for _, file := range files {
			f, err := file.Open()
			if err != nil {
				continue
			}
			defer f.Close()

			data, err := io.ReadAll(f)
			if err != nil {
				continue
			}

			media = append(media, &Media{
				ID:       generateID(),
				Name:     file.Filename,
				MimeType: file.Header.Get("Content-Type"),
				Size:     file.Size,
				Data:     data,
			})
		}
	}

	return media, nil
}

func (m *Media) ToJSON() (string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FromBase64(data string) (*Media, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}

	return &Media{
		ID:     generateID(),
		Data:   decoded,
		Base64: data,
		Size:   int64(len(decoded)),
	}, nil
}

type Converter struct{}

func NewConverter() *Converter {
	return &Converter{}
}

func (c *Converter) ImageToPNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Converter) Resize(img image.Image, width, height int) image.Image {
	return img
}
