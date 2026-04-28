package speech

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
)

type openAIAudioAPIClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func newOpenAIAudioAPIClient(apiKey, baseURL string, client *http.Client) *openAIAudioAPIClient {
	return &openAIAudioAPIClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  client,
	}
}

func (c *openAIAudioAPIClient) DoTranscriptionRequest(ctx context.Context, endpoint string, audio []byte, options TranscribeOptions, stream bool) (*http.Response, error) {
	body, contentType, err := c.buildMultipartBody(audio, options, stream)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, body)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "anyclaw-stt/1.0")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: request failed: %v", err)
	}

	return resp, nil
}

func (c *openAIAudioAPIClient) buildMultipartBody(audio []byte, options TranscribeOptions, stream bool) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	filename := "audio." + string(options.InputFormat)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to create form file: %v", err)
	}

	if _, err := part.Write(audio); err != nil {
		return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write audio data: %v", err)
	}

	if err := writer.WriteField("model", options.Model); err != nil {
		return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write model field: %v", err)
	}

	if options.Language != "" {
		if err := writer.WriteField("language", options.Language); err != nil {
			return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write language field: %v", err)
		}
	}

	if options.Prompt != "" {
		if err := writer.WriteField("prompt", options.Prompt); err != nil {
			return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write prompt field: %v", err)
		}
	}

	if options.Temperature > 0 {
		if err := writer.WriteField("temperature", fmt.Sprintf("%.2f", options.Temperature)); err != nil {
			return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write temperature field: %v", err)
		}
	}

	if options.MaxAlternatives > 0 {
		if err := writer.WriteField("max_alternatives", fmt.Sprintf("%d", options.MaxAlternatives)); err != nil {
			return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write max_alternatives field: %v", err)
		}
	}

	// Streaming requests use response_format=json and should not send
	// verbose-only timestamp granularities.
	if !stream && (options.WordTimestamps || options.SpeakerLabels) {
		if options.WordTimestamps {
			if err := writer.WriteField("timestamp_granularities[]", "word"); err != nil {
				return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write word timestamp_granularities: %v", err)
			}
		}
		if options.SpeakerLabels {
			if err := writer.WriteField("timestamp_granularities[]", "segment"); err != nil {
				return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write segment timestamp_granularities: %v", err)
			}
		}
	}

	responseType := "verbose_json"
	if stream {
		responseType = "json"
	}
	if err := writer.WriteField("response_format", responseType); err != nil {
		return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write response_format field: %v", err)
	}

	if stream {
		if err := writer.WriteField("stream", "true"); err != nil {
			return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write stream field: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to close multipart writer: %v", err)
	}

	return &body, writer.FormDataContentType(), nil
}

