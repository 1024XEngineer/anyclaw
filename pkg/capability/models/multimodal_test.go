package llm

import (
	"testing"
)

func TestMessage_AppendText(t *testing.T) {
	m := Message{Role: "user"}
	m.AppendText("Hello")
	m.AppendText(" World")

	blocks, err := m.UnmarshalContent()
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Text != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", blocks[0].Text)
	}
}

func TestMessage_AppendImageURL(t *testing.T) {
	m := Message{Role: "user"}
	m.AppendText("Describe this:")
	m.AppendImageURL("https://example.com/img.jpg", "high")

	blocks, err := m.UnmarshalContent()
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != ContentTypeText {
		t.Errorf("expected text block, got %s", blocks[0].Type)
	}
	if blocks[1].Type != ContentTypeImageURL {
		t.Errorf("expected image_url block, got %s", blocks[1].Type)
	}
	if blocks[1].ImageURL.URL != "https://example.com/img.jpg" {
		t.Errorf("wrong URL: %s", blocks[1].ImageURL.URL)
	}
	if blocks[1].ImageURL.Detail != "high" {
		t.Errorf("wrong detail: %s", blocks[1].ImageURL.Detail)
	}
}

func TestMessage_AppendImageBase64(t *testing.T) {
	m := Message{Role: "user"}
	m.AppendText("What is this?")
	m.AppendImageBase64([]byte("fake-image-data"), "image/png")

	blocks, err := m.UnmarshalContent()
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[1].Type != ContentTypeImage {
		t.Errorf("expected image block, got %s", blocks[1].Type)
	}
	if blocks[1].Image.Source.MediaType != "image/png" {
		t.Errorf("wrong media type: %s", blocks[1].Image.Source.MediaType)
	}
}

func TestMessage_SetContentBlocks(t *testing.T) {
	m := Message{Role: "user"}
	m.SetContentBlocks([]ContentBlock{
		{Type: ContentTypeText, Text: "Hello"},
		{Type: ContentTypeImageURL, ImageURL: &ImageURLBlock{URL: "https://example.com/img.jpg"}},
	})

	if m.Content != "" {
		t.Error("Content should be empty for multi-block messages")
	}
	if m.contentBlocks == nil || len(m.contentBlocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(m.contentBlocks))
	}
}

func TestNewUserMessage(t *testing.T) {
	m := NewUserMessage(
		TextBlock("Describe this image:"),
		ImageURLBlockFromURL("https://example.com/test.jpg", "auto"),
	)

	if m.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", m.Role)
	}
	if m.contentBlocks == nil || len(m.contentBlocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(m.contentBlocks))
	}
}

func TestTextBlock(t *testing.T) {
	b := TextBlock("Hello")
	if b.Type != ContentTypeText {
		t.Errorf("expected text type, got %s", b.Type)
	}
	if b.Text != "Hello" {
		t.Errorf("expected 'Hello', got '%s'", b.Text)
	}
}

func TestImageBlockFromBase64(t *testing.T) {
	b := ImageBlockFromBase64([]byte("data"), "image/jpeg")
	if b.Type != ContentTypeImage {
		t.Errorf("expected image type, got %s", b.Type)
	}
	if b.Image.Source.Type != "base64" {
		t.Errorf("expected base64 source type, got %s", b.Image.Source.Type)
	}
	if b.Image.Source.MediaType != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", b.Image.Source.MediaType)
	}
}

func TestMimeTypeFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"icon.png", "image/png"},
		{"anim.gif", "image/gif"},
		{"pic.webp", "image/webp"},
		{"scan.bmp", "image/bmp"},
		{"unknown.xyz", ""},
	}

	for _, tt := range tests {
		got := mimeTypeFromPath(tt.path)
		if got != tt.want {
			t.Errorf("mimeTypeFromPath(%s) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestIsVisionCapableModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gpt-4o", true},
		{"gpt-4o-mini", true},
		{"gpt-4-turbo", true},
		{"gpt-4-vision-preview", true},
		{"claude-3-sonnet-20240229", true},
		{"claude-3-opus-20240229", true},
		{"claude-3-haiku-20240307", true},
		{"claude-3-5-sonnet-20241022", true},
		{"gemini-1.5-pro", true},
		{"gemini-1.5-flash", true},
		{"qwen2.5-vl-72b", true},
		{"gpt-3.5-turbo", false},
		{"gpt-4", false},
		{"llama-3-70b", false},
		{"mistral-large", false},
	}

	for _, tt := range tests {
		got := IsVisionCapableModel(tt.model)
		if got != tt.want {
			t.Errorf("IsVisionCapableModel(%s) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestSerializeMessagesOpenAI_TextOnly(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
	}

	serialized := serializeMessagesOpenAI(messages)

	if len(serialized) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(serialized))
	}
	if serialized[0]["role"] != "system" {
		t.Errorf("expected system role, got %v", serialized[0]["role"])
	}
	if serialized[0]["content"] != "You are helpful" {
		t.Errorf("wrong content: %v", serialized[0]["content"])
	}
	if serialized[1]["content"] != "Hello" {
		t.Errorf("wrong content: %v", serialized[1]["content"])
	}
}

func TestSerializeMessagesOpenAI_Multimodal(t *testing.T) {
	m := NewUserMessage(
		TextBlock("What's in this image?"),
		ImageURLBlockFromURL("https://example.com/test.jpg", "auto"),
	)

	serialized := serializeMessagesOpenAI([]Message{m})

	if len(serialized) != 1 {
		t.Fatalf("expected 1 message, got %d", len(serialized))
	}

	content, ok := serialized[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be array, got %T", serialized[0]["content"])
	}

	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(content))
	}

	if content[0]["type"] != "text" {
		t.Errorf("expected text type, got %v", content[0]["type"])
	}
	if content[1]["type"] != "image_url" {
		t.Errorf("expected image_url type, got %v", content[1]["type"])
	}
}

func TestSerializeMessagesOpenAI_Base64Image(t *testing.T) {
	m := NewUserMessage(
		TextBlock("Describe:"),
		ImageBlockFromBase64([]byte("img-data"), "image/png"),
	)

	serialized := serializeMessagesOpenAI([]Message{m})

	content := serialized[0]["content"].([]map[string]any)
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(content))
	}

	imgBlock := content[1]["image_url"].(map[string]any)
	url := imgBlock["url"].(string)
	if url != "data:image/png;base64,aW1nLWRhdGE=" {
		t.Errorf("wrong data URL: %s", url)
	}
}

func TestSerializeMessagesAnthropic_TextOnly(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
	}

	serialized, systemPrompt := serializeMessagesAnthropic(messages)

	if len(serialized) != 1 {
		t.Fatalf("expected 1 message, got %d", len(serialized))
	}
	if systemPrompt != "You are helpful" {
		t.Errorf("expected system prompt 'You are helpful', got '%s'", systemPrompt)
	}
	if serialized[0]["content"] != "Hello" {
		t.Errorf("wrong content: %v", serialized[0]["content"])
	}
}

func TestSerializeMessagesAnthropic_Multimodal(t *testing.T) {
	m := NewUserMessage(
		TextBlock("What is this?"),
		ImageBlockFromBase64([]byte("img-data"), "image/png"),
	)

	serialized, _ := serializeMessagesAnthropic([]Message{m})

	if len(serialized) != 1 {
		t.Fatalf("expected 1 message, got %d", len(serialized))
	}

	content := serialized[0]["content"].([]map[string]any)
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(content))
	}

	if content[1]["type"] != "image" {
		t.Errorf("expected image type, got %v", content[1]["type"])
	}

	source := content[1]["source"].(map[string]any)
	if source["type"] != "base64" {
		t.Errorf("expected base64 source, got %v", source["type"])
	}
	if source["media_type"] != "image/png" {
		t.Errorf("expected image/png, got %v", source["media_type"])
	}
}

func TestContentBlock_RoundTrip(t *testing.T) {
	m := Message{Role: "user"}
	m.AppendText("Test")
	m.AppendImageURL("https://example.com/img.jpg", "")
	m.AppendText(" More text")

	blocks, err := m.UnmarshalContent()
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	if blocks[0].Text != "Test" {
		t.Errorf("expected 'Test', got '%s'", blocks[0].Text)
	}
	if blocks[2].Text != " More text" {
		t.Errorf("expected ' More text', got '%s'", blocks[2].Text)
	}
}

func TestImageBlockFromFile_NotFound(t *testing.T) {
	_, err := ImageBlockFromFile("/nonexistent/file.jpg")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestMessage_AppendImageFile_NotFound(t *testing.T) {
	m := Message{Role: "user"}
	err := m.AppendImageFile("/nonexistent/file.jpg")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
