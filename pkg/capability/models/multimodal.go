package llm

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeImageURL ContentType = "image_url"
	ContentTypeImage    ContentType = "image"
	ContentTypeFile     ContentType = "file"
)

type ContentBlock struct {
	Type     ContentType    `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL *ImageURLBlock `json:"image_url,omitempty"`
	Image    *ImageBlock    `json:"image,omitempty"`
	File     *FileBlock     `json:"file,omitempty"`
}

type ImageURLBlock struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type ImageBlock struct {
	Source ImageSource `json:"source"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type FileBlock struct {
	Filename string `json:"filename"`
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
}

func (m *Message) UnmarshalContent() ([]ContentBlock, error) {
	if m.contentBlocks != nil && len(m.contentBlocks) > 0 {
		return m.contentBlocks, nil
	}

	if m.Content == "" {
		return nil, nil
	}

	return []ContentBlock{
		{Type: ContentTypeText, Text: m.Content},
	}, nil
}

func (m *Message) SetContentBlocks(blocks []ContentBlock) {
	m.contentBlocks = blocks
	if len(blocks) == 1 && blocks[0].Type == ContentTypeText {
		m.Content = blocks[0].Text
	} else {
		m.Content = ""
	}
}

func (m *Message) AppendText(text string) {
	if m.contentBlocks == nil || len(m.contentBlocks) == 0 {
		if m.Content != "" {
			m.contentBlocks = []ContentBlock{{Type: ContentTypeText, Text: m.Content}}
		} else {
			m.contentBlocks = []ContentBlock{}
		}
	}

	if len(m.contentBlocks) > 0 && m.contentBlocks[len(m.contentBlocks)-1].Type == ContentTypeText {
		m.contentBlocks[len(m.contentBlocks)-1].Text += text
	} else {
		m.contentBlocks = append(m.contentBlocks, ContentBlock{Type: ContentTypeText, Text: text})
	}
}

func (m *Message) AppendImageURL(url string, detail string) {
	block := ContentBlock{
		Type:     ContentTypeImageURL,
		ImageURL: &ImageURLBlock{URL: url},
	}
	if detail != "" {
		block.ImageURL.Detail = detail
	}
	m.contentBlocks = append(m.contentBlocks, block)
}

func (m *Message) AppendImageBase64(data []byte, mimeType string) {
	encoded := base64.StdEncoding.EncodeToString(data)
	m.contentBlocks = append(m.contentBlocks, ContentBlock{
		Type: ContentTypeImage,
		Image: &ImageBlock{
			Source: ImageSource{
				Type:      "base64",
				MediaType: mimeType,
				Data:      encoded,
			},
		},
	})
}

func (m *Message) AppendImageFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read image file: %w", err)
	}

	mimeType := mimeTypeFromPath(path)
	if mimeType == "" {
		mimeType = "image/png"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	m.contentBlocks = append(m.contentBlocks, ContentBlock{
		Type: ContentTypeImage,
		Image: &ImageBlock{
			Source: ImageSource{
				Type:      "base64",
				MediaType: mimeType,
				Data:      encoded,
			},
		},
	})
	return nil
}

func NewUserMessage(parts ...ContentBlock) Message {
	m := Message{Role: "user"}
	if len(parts) > 0 {
		m.SetContentBlocks(parts)
	}
	return m
}

func NewTextMessage(role, text string) Message {
	return Message{Role: role, Content: text}
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: ContentTypeText, Text: text}
}

func ImageURLBlockFromURL(url string, detail string) ContentBlock {
	block := ContentBlock{Type: ContentTypeImageURL, ImageURL: &ImageURLBlock{URL: url}}
	if detail != "" {
		block.ImageURL.Detail = detail
	}
	return block
}

func ImageBlockFromBase64(data []byte, mimeType string) ContentBlock {
	return ContentBlock{
		Type: ContentTypeImage,
		Image: &ImageBlock{
			Source: ImageSource{
				Type:      "base64",
				MediaType: mimeType,
				Data:      base64.StdEncoding.EncodeToString(data),
			},
		},
	}
}

func ImageBlockFromFile(path string) (ContentBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ContentBlock{}, fmt.Errorf("read image file: %w", err)
	}

	mimeType := mimeTypeFromPath(path)
	if mimeType == "" {
		mimeType = "image/png"
	}

	return ContentBlock{
		Type: ContentTypeImage,
		Image: &ImageBlock{
			Source: ImageSource{
				Type:      "base64",
				MediaType: mimeType,
				Data:      base64.StdEncoding.EncodeToString(data),
			},
		},
	}, nil
}

func mimeTypeFromPath(path string) string {
	ext := strings.ToLower(path)
	switch {
	case strings.HasSuffix(ext, ".jpg"), strings.HasSuffix(ext, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(ext, ".png"):
		return "image/png"
	case strings.HasSuffix(ext, ".gif"):
		return "image/gif"
	case strings.HasSuffix(ext, ".webp"):
		return "image/webp"
	case strings.HasSuffix(ext, ".bmp"):
		return "image/bmp"
	default:
		return ""
	}
}

func IsVisionCapableModel(model string) bool {
	model = strings.ToLower(model)

	visionModels := []string{
		"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-4-vision",
		"claude-3-sonnet", "claude-3-haiku", "claude-3-opus",
		"claude-3-5-sonnet", "claude-3-5-haiku", "claude-3-7-sonnet",
		"gemini-1.5-pro", "gemini-1.5-flash", "gemini-2.0",
		"qwen-vl", "qwen2-vl", "qwen2.5-vl",
		"llava", "moondream", "bakllava",
		"pixtral",
	}

	for _, vm := range visionModels {
		if strings.Contains(model, vm) {
			return true
		}
	}

	return false
}
