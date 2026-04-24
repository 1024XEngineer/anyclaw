package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type TelegramAudioSender struct {
	botToken string
	baseURL  string
	client   *http.Client
}

func NewTelegramAudioSender(botToken string) *TelegramAudioSender {
	return &TelegramAudioSender{
		botToken: botToken,
		baseURL:  "https://api.telegram.org/bot" + botToken,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *TelegramAudioSender) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	if recipient == "" {
		return "", fmt.Errorf("telegram: recipient chat ID is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	audioPart, err := writer.CreateFormFile("audio", "tts.mp3")
	if err != nil {
		return "", fmt.Errorf("telegram: failed to create form file: %w", err)
	}
	audioPart.Write(audio.Data)

	writer.WriteField("chat_id", recipient)
	if caption != "" {
		writer.WriteField("caption", caption)
	}
	writer.Close()

	url := s.baseURL + "/sendAudio"
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("telegram: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("telegram: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Ok     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("telegram: failed to decode response: %w", err)
	}

	if !result.Ok {
		return "", fmt.Errorf("telegram: API returned ok=false: %s", string(respBody))
	}

	return fmt.Sprintf("%d", result.Result.MessageID), nil
}

func (s *TelegramAudioSender) CanSend(channel string) bool {
	return channel == "telegram"
}

type DiscordAudioSender struct {
	botToken string
	apiBase  string
	client   *http.Client
}

func NewDiscordAudioSender(botToken string) *DiscordAudioSender {
	return &DiscordAudioSender{
		botToken: botToken,
		apiBase:  "https://discord.com/api/v10",
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *DiscordAudioSender) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	if recipient == "" {
		return "", fmt.Errorf("discord: channel ID is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	filePart, err := writer.CreateFormFile("files[0]", "tts.mp3")
	if err != nil {
		return "", fmt.Errorf("discord: failed to create form file: %w", err)
	}
	filePart.Write(audio.Data)

	payload := map[string]any{
		"content": caption,
	}
	payloadJSON, _ := json.Marshal(payload)
	writer.WriteField("payload_json", string(payloadJSON))
	writer.Close()

	url := s.apiBase + "/channels/" + recipient + "/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("discord: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+s.botToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("discord: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discord: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("discord: failed to decode response: %w", err)
	}

	return result.ID, nil
}

func (s *DiscordAudioSender) CanSend(channel string) bool {
	return channel == "discord"
}

type SlackAudioSender struct {
	botToken string
	baseURL  string
	client   *http.Client
}

func NewSlackAudioSender(botToken string) *SlackAudioSender {
	return &SlackAudioSender{
		botToken: botToken,
		baseURL:  "https://slack.com/api",
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *SlackAudioSender) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	if recipient == "" {
		return "", fmt.Errorf("slack: channel ID is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	filePart, err := writer.CreateFormFile("file", "tts.mp3")
	if err != nil {
		return "", fmt.Errorf("slack: failed to create form file: %w", err)
	}
	filePart.Write(audio.Data)

	writer.WriteField("channels", recipient)
	writer.WriteField("filetype", "mp3")
	writer.WriteField("initial_comment", caption)
	writer.WriteField("filename", "tts.mp3")
	writer.Close()

	url := s.baseURL + "/files.upload"
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("slack: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.botToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slack: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("slack: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Ok   bool `json:"ok"`
		File struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"file"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("slack: failed to decode response: %w", err)
	}

	if !result.Ok {
		return "", fmt.Errorf("slack: API error: %s", result.Error)
	}

	return result.File.ID, nil
}

func (s *SlackAudioSender) CanSend(channel string) bool {
	return channel == "slack"
}

type WhatsAppAudioSender struct {
	phoneNumberID string
	accessToken   string
	baseURL       string
	client        *http.Client
}

func NewWhatsAppAudioSender(phoneNumberID, accessToken string) *WhatsAppAudioSender {
	return &WhatsAppAudioSender{
		phoneNumberID: phoneNumberID,
		accessToken:   accessToken,
		baseURL:       "https://graph.facebook.com/v17.0",
		client:        &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *WhatsAppAudioSender) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	if recipient == "" {
		return "", fmt.Errorf("whatsapp: recipient phone ID is required")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	filePart, err := writer.CreateFormFile("file", "tts.mp3")
	if err != nil {
		return "", fmt.Errorf("whatsapp: failed to create form file: %w", err)
	}
	filePart.Write(audio.Data)

	writer.WriteField("messaging_product", "whatsapp")
	writer.WriteField("type", "audio")
	writer.Close()

	url := fmt.Sprintf("%s/%s/media", s.baseURL, s.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("whatsapp: failed to create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.accessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("whatsapp: upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whatsapp: upload API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var uploadResult struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &uploadResult); err != nil {
		return "", fmt.Errorf("whatsapp: failed to decode upload response: %w", err)
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                recipient,
		"type":              "audio",
		"audio": map[string]any{
			"id": uploadResult.ID,
		},
	}
	payloadBody, _ := json.Marshal(payload)

	msgURL := fmt.Sprintf("%s/%s/messages", s.baseURL, s.phoneNumberID)
	msgReq, err := http.NewRequestWithContext(ctx, "POST", msgURL, bytes.NewReader(payloadBody))
	if err != nil {
		return "", fmt.Errorf("whatsapp: failed to create send request: %w", err)
	}
	msgReq.Header.Set("Authorization", "Bearer "+s.accessToken)
	msgReq.Header.Set("Content-Type", "application/json")

	msgResp, err := s.client.Do(msgReq)
	if err != nil {
		return "", fmt.Errorf("whatsapp: send request failed: %w", err)
	}
	defer msgResp.Body.Close()

	msgRespBody, _ := io.ReadAll(msgResp.Body)
	if msgResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whatsapp: send API error (%d): %s", msgResp.StatusCode, string(msgRespBody))
	}

	var sendResult struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(msgRespBody, &sendResult); err != nil {
		return "", fmt.Errorf("whatsapp: failed to decode send response: %w", err)
	}

	if len(sendResult.Messages) == 0 {
		return "", fmt.Errorf("whatsapp: no message ID in response")
	}

	return sendResult.Messages[0].ID, nil
}

func (s *WhatsAppAudioSender) CanSend(channel string) bool {
	return channel == "whatsapp"
}

type SignalAudioSender struct {
	baseURL string
	number  string
	client  *http.Client
}

func NewSignalAudioSender(baseURL, number string) *SignalAudioSender {
	return &SignalAudioSender{
		baseURL: baseURL,
		number:  number,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *SignalAudioSender) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	if recipient == "" {
		return "", fmt.Errorf("signal: recipient is required")
	}

	payload := map[string]any{
		"message":      caption,
		"number":       s.number,
		"recipients":   []string{recipient},
		"base64_audio": fmt.Sprintf("data:audio/mpeg;base64,%s", AudioToBase64(audio.Data)),
	}
	body, _ := json.Marshal(payload)

	url := s.baseURL + "/v2/send"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("signal: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("signal: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("signal: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Timestamp int64 `json:"timestamp"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("signal: failed to decode response: %w", err)
	}

	return fmt.Sprintf("%d", result.Timestamp), nil
}

func (s *SignalAudioSender) CanSend(channel string) bool {
	return channel == "signal"
}

type GenericWebhookAudioSender struct {
	webhookURL string
	headers    map[string]string
	client     *http.Client
}

func NewGenericWebhookAudioSender(webhookURL string, headers map[string]string) *GenericWebhookAudioSender {
	return &GenericWebhookAudioSender{
		webhookURL: webhookURL,
		headers:    headers,
		client:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *GenericWebhookAudioSender) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	payload := map[string]any{
		"channel":    channel,
		"recipient":  recipient,
		"caption":    caption,
		"audio":      AudioToBase64(audio.Data),
		"format":     string(audio.Format),
		"sampleRate": audio.SampleRate,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("webhook: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webhook: error (%d): %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

func (s *GenericWebhookAudioSender) CanSend(channel string) bool {
	return true
}
