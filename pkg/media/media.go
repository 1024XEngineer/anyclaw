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
	detector     *Detector
	transcoder   *Transcoder
	storage      StorageBackend
	signer       *URLSigner
}

func NewProcessor(storagePath string) *Processor {
	p := &Processor{
		storagePath:  storagePath,
		maxFileSize:  10 * 1024 * 1024,
		allowedTypes: []Type{TypeImage, TypeAudio, TypeVideo, TypeDoc},
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		detector:     NewDetector(),
		transcoder:   NewTranscoder(),
	}

	if storagePath != "" {
		p.storage = NewLocalStorage(LocalStorageConfig{
			BasePath: storagePath,
		})
	}

	return p
}

func (p *Processor) SetStorage(backend StorageBackend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.storage = backend
}

func (p *Processor) Storage() StorageBackend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.storage
}

func (p *Processor) SetTranscoder(t *Transcoder) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.transcoder = t
}

func (p *Processor) Transcoder() *Transcoder {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.transcoder
}

func (p *Processor) SetDetector(d *Detector) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.detector = d
}

func (p *Processor) Detector() *Detector {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.detector
}

func (p *Processor) Process(ctx context.Context, input *Media) (*Media, error) {
	p.mu.RLock()
	detector := p.detector
	p.mu.RUnlock()

	if detector != nil && len(input.Data) > 0 {
		mediaType := detector.Detect(input.Data, input.Name, input.MimeType)
		if mediaType.Format != FormatUnknown {
			input.Type = mediaType.Type
			input.MimeType = mediaType.MimeType
			if input.Metadata == nil {
				input.Metadata = make(map[string]any)
			}
			input.Metadata["format"] = string(mediaType.Format)
		}
	}

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
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}

		if meta, err := ExtractImageMetadata(input.Data); err == nil {
			for k, v := range meta {
				input.Metadata[k] = v
			}
		}
	}

	return input, nil
}

func (p *Processor) processAudio(ctx context.Context, input *Media) (*Media, error) {
	if len(input.Data) > 0 {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}
		for k, v := range ExtractAudioMetadata(input.Data) {
			input.Metadata[k] = v
		}
	}
	return input, nil
}

func (p *Processor) processVideo(ctx context.Context, input *Media) (*Media, error) {
	if len(input.Data) > 0 {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}
		for k, v := range ExtractVideoMetadata(input.Data) {
			input.Metadata[k] = v
		}
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
	p.mu.RLock()
	storage := p.storage
	p.mu.RUnlock()

	if storage == nil {
		return "", fmt.Errorf("no storage backend configured")
	}

	ext := extensionFromMime(media.MimeType)
	key := fmt.Sprintf("%s%s", media.ID, ext)

	opts := StoragePutOptions{
		MimeType: media.MimeType,
		Metadata: map[string]string{
			"media-type": string(media.Type),
			"media-name": media.Name,
		},
	}

	obj, err := storage.Put(ctx, key, media.Data, opts)
	if err != nil {
		return "", fmt.Errorf("upload to storage: %w", err)
	}

	media.Path = obj.URL
	media.URL = obj.URL

	return obj.URL, nil
}

func (p *Processor) Save(ctx context.Context, media *Media) (string, error) {
	p.mu.RLock()
	storage := p.storage
	p.mu.RUnlock()

	if storage == nil {
		return "", fmt.Errorf("no storage backend configured")
	}

	key := media.Name
	if key == "" {
		ext := extensionFromMime(media.MimeType)
		key = fmt.Sprintf("%s%s", media.ID, ext)
	}

	opts := StoragePutOptions{
		MimeType: media.MimeType,
	}

	obj, err := storage.Put(ctx, key, media.Data, opts)
	if err != nil {
		return "", fmt.Errorf("save to storage: %w", err)
	}

	media.Path = obj.URL
	media.URL = obj.URL

	return obj.URL, nil
}

func (p *Processor) Load(ctx context.Context, key string) (*Media, error) {
	p.mu.RLock()
	storage := p.storage
	p.mu.RUnlock()

	if storage == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}

	obj, err := storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load from storage: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(obj.Metadata["data"])
	if err != nil {
		return nil, fmt.Errorf("decode stored data: %w", err)
	}

	return &Media{
		ID:       generateID(),
		Type:     p.guessType(obj.MimeType),
		Name:     filepath.Base(key),
		Size:     obj.Size,
		MimeType: obj.MimeType,
		Data:     data,
		Base64:   base64.StdEncoding.EncodeToString(data),
		Path:     obj.URL,
		URL:      obj.URL,
	}, nil
}

func (p *Processor) Delete(ctx context.Context, key string) error {
	p.mu.RLock()
	storage := p.storage
	p.mu.RUnlock()

	if storage == nil {
		return fmt.Errorf("no storage backend configured")
	}

	return storage.Delete(ctx, key)
}

func (p *Processor) Compress(ctx context.Context, media *Media, opts ImageOptions) (*Media, error) {
	p.mu.RLock()
	transcoder := p.transcoder
	p.mu.RUnlock()

	if transcoder == nil {
		return nil, fmt.Errorf("no transcoder configured")
	}

	if len(media.Data) == 0 && media.Path != "" {
		data, err := os.ReadFile(media.Path)
		if err != nil {
			return nil, fmt.Errorf("read media file: %w", err)
		}
		media.Data = data
	}

	var result []byte
	var err error

	switch media.Type {
	case TypeImage:
		result, err = transcoder.CompressImage(media.Data, opts)
	case TypeAudio:
		audioOpts := DefaultAudioOptions()
		if opts.Format != FormatUnknown {
			audioOpts.Format = opts.Format
		}
		if opts.Quality > 0 {
			audioOpts.Bitrate = opts.Quality
		}
		result, err = transcoder.TranscodeAudio(ctx, media.Data, audioOpts)
	case TypeVideo:
		videoOpts := DefaultVideoOptions()
		if opts.Format != FormatUnknown {
			videoOpts.Format = opts.Format
		}
		if opts.Quality > 0 {
			videoOpts.CRF = 51 - opts.Quality/2
		}
		result, err = transcoder.TranscodeVideo(ctx, media.Data, videoOpts)
	default:
		return nil, fmt.Errorf("unsupported media type for compression: %s", media.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("compress media: %w", err)
	}

	media.Data = result
	media.Size = int64(len(result))
	media.Base64 = base64.StdEncoding.EncodeToString(result)

	detected := DetectMediaType(result, media.Name, media.MimeType)
	if detected.Format != FormatUnknown {
		media.Type = detected.Type
		media.MimeType = detected.MimeType
		if media.Metadata == nil {
			media.Metadata = make(map[string]any)
		}
		media.Metadata["format"] = string(detected.Format)
	}

	return media, nil
}

func (p *Processor) Convert(ctx context.Context, media *Media, targetFormat Format) (*Media, error) {
	p.mu.RLock()
	transcoder := p.transcoder
	p.mu.RUnlock()

	if transcoder == nil {
		return nil, fmt.Errorf("no transcoder configured")
	}

	if len(media.Data) == 0 && media.Path != "" {
		data, err := os.ReadFile(media.Path)
		if err != nil {
			return nil, fmt.Errorf("read media file: %w", err)
		}
		media.Data = data
	}

	var result []byte
	var err error

	switch media.Type {
	case TypeImage:
		result, err = transcoder.ConvertImage(media.Data, targetFormat)
	case TypeAudio:
		audioOpts := DefaultAudioOptions()
		audioOpts.Format = targetFormat
		result, err = transcoder.TranscodeAudio(ctx, media.Data, audioOpts)
	case TypeVideo:
		videoOpts := DefaultVideoOptions()
		videoOpts.Format = targetFormat
		result, err = transcoder.TranscodeVideo(ctx, media.Data, videoOpts)
	default:
		return nil, fmt.Errorf("unsupported media type for conversion: %s", media.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("convert media: %w", err)
	}

	media.Data = result
	media.Size = int64(len(result))
	media.Base64 = base64.StdEncoding.EncodeToString(result)
	media.MimeType = formatToMIME(targetFormat)

	if media.Metadata == nil {
		media.Metadata = make(map[string]any)
	}
	media.Metadata["format"] = string(targetFormat)

	return media, nil
}

func (p *Processor) Exists(ctx context.Context, key string) (bool, error) {
	p.mu.RLock()
	storage := p.storage
	p.mu.RUnlock()

	if storage == nil {
		return false, fmt.Errorf("no storage backend configured")
	}

	return storage.Exists(ctx, key)
}

func (p *Processor) List(ctx context.Context, prefix string) ([]*StorageObject, error) {
	p.mu.RLock()
	storage := p.storage
	p.mu.RUnlock()

	if storage == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}

	return storage.List(ctx, prefix)
}

func (p *Processor) PresignURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	p.mu.RLock()
	storage := p.storage
	signer := p.signer
	p.mu.RUnlock()

	if storage == nil {
		return "", fmt.Errorf("no storage backend configured")
	}

	if signer != nil {
		return signer.SignStorageObject(storage, key, expires)
	}

	return storage.URL(ctx, key, expires)
}

func (p *Processor) SetSigner(signer *URLSigner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.signer = signer
}

func (p *Processor) Signer() *URLSigner {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.signer
}

func (p *Processor) VerifySignedURL(ctx context.Context, signedURL string) (*SignedURLResult, error) {
	p.mu.RLock()
	signer := p.signer
	p.mu.RUnlock()

	if signer == nil {
		return nil, fmt.Errorf("no URL signer configured")
	}

	baseURL, expireTime, metadata, err := signer.VerifyURL(signedURL)
	if err != nil {
		return nil, err
	}

	remaining := expireTime.Sub(time.Now().UTC())

	return &SignedURLResult{
		URL:       baseURL,
		ExpiresAt: expireTime,
		Remaining: remaining,
		Key:       "",
		Metadata:  metadata,
		IsRevoked: remaining < 0,
	}, nil
}

func (p *Processor) RevokeSignedURL(ctx context.Context, signedURL string) error {
	p.mu.RLock()
	signer := p.signer
	p.mu.RUnlock()

	if signer == nil {
		return fmt.Errorf("no URL signer configured")
	}

	return signer.RevokeURL(signedURL)
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
