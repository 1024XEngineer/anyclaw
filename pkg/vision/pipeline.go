package vision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/llm"
)

type MediaUnderstandingConfig struct {
	VisionProvider    VisionProvider
	AudioAnalyzer     *AudioAnalyzer
	KeyFrameExtractor *KeyFrameExtractor
	MaxImageSize      int
	MaxVideoDuration  float64
	Timeout           time.Duration
}

func DefaultMediaUnderstandingConfig() MediaUnderstandingConfig {
	return MediaUnderstandingConfig{
		MaxImageSize:     10 * 1024 * 1024,
		MaxVideoDuration: 300,
		Timeout:          60 * time.Second,
	}
}

type MediaUnderstandingResult struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Summary     string         `json:"summary"`
	Details     any            `json:"details"`
	Metadata    map[string]any `json:"metadata"`
}

type MediaUnderstandingPipeline struct {
	cfg MediaUnderstandingConfig
}

func NewMediaUnderstandingPipeline(cfg MediaUnderstandingConfig) *MediaUnderstandingPipeline {
	return &MediaUnderstandingPipeline{cfg: cfg}
}

func (p *MediaUnderstandingPipeline) UnderstandImage(ctx context.Context, imageData []byte, mimeType string) (*MediaUnderstandingResult, error) {
	if len(imageData) > p.cfg.MaxImageSize {
		imageData = imageData[:p.cfg.MaxImageSize]
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	result := &MediaUnderstandingResult{
		Type:     "image",
		Metadata: map[string]any{},
	}

	if p.cfg.VisionProvider != nil {
		analysis, err := p.cfg.VisionProvider.AnalyzeImage(ctx, imageData, mimeType)
		if err != nil {
			return nil, fmt.Errorf("vision analysis: %w", err)
		}

		result.Description = analysis.Description
		result.Details = analysis
		result.Metadata["labels"] = len(analysis.Labels)
		result.Metadata["objects"] = len(analysis.Objects)
		result.Metadata["text_regions"] = len(analysis.Text)
		result.Metadata["faces"] = len(analysis.Faces)

		if len(analysis.Labels) > 0 {
			labels := make([]string, 0, len(analysis.Labels))
			for _, l := range analysis.Labels {
				labels = append(labels, l.Name)
			}
			result.Summary = strings.Join(labels, ", ")
		}
	} else {
		result.Description = "No vision provider configured"
	}

	return result, nil
}

func (p *MediaUnderstandingPipeline) UnderstandImageFile(ctx context.Context, path string) (*MediaUnderstandingResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read image file: %w", err)
	}

	mimeType := mimeTypeFromPath(path)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	return p.UnderstandImage(ctx, data, mimeType)
}

func (p *MediaUnderstandingPipeline) UnderstandImageURL(ctx context.Context, imageURL string) (*MediaUnderstandingResult, error) {
	if p.cfg.VisionProvider == nil {
		return nil, fmt.Errorf("no vision provider configured")
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	analysis, err := p.cfg.VisionProvider.AnalyzeImageURL(ctx, imageURL)
	if err != nil {
		return nil, fmt.Errorf("vision analysis from URL: %w", err)
	}

	result := &MediaUnderstandingResult{
		Type:        "image",
		Description: analysis.Description,
		Details:     analysis,
		Metadata:    map[string]any{},
	}

	if len(analysis.Labels) > 0 {
		labels := make([]string, 0, len(analysis.Labels))
		for _, l := range analysis.Labels {
			labels = append(labels, l.Name)
		}
		result.Summary = strings.Join(labels, ", ")
	}

	return result, nil
}

func (p *MediaUnderstandingPipeline) UnderstandVideo(ctx context.Context, videoData []byte) (*MediaUnderstandingResult, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	result := &MediaUnderstandingResult{
		Type:     "video",
		Metadata: map[string]any{},
	}

	if p.cfg.KeyFrameExtractor == nil {
		p.cfg.KeyFrameExtractor = NewKeyFrameExtractor()
	}

	videoAnalysis, err := p.cfg.KeyFrameExtractor.ExtractKeyFrames(ctx, videoData)
	if err != nil {
		return nil, fmt.Errorf("keyframe extraction: %w", err)
	}

	result.Details = videoAnalysis
	result.Metadata["duration"] = videoAnalysis.Duration
	result.Metadata["scenes"] = len(videoAnalysis.Scenes)
	result.Metadata["key_frames"] = len(videoAnalysis.KeyFrames)
	result.Metadata["resolution"] = fmt.Sprintf("%dx%d", videoAnalysis.Width, videoAnalysis.Height)
	result.Metadata["codec"] = videoAnalysis.Codec

	if len(videoAnalysis.Scenes) > 0 && p.cfg.VisionProvider != nil {
		var sceneDescriptions []string
		for i, scene := range videoAnalysis.Scenes {
			if i >= 5 {
				break
			}
			if len(scene.KeyFrames) > 0 {
				kf := scene.KeyFrames[0]
				if len(kf.FrameData) > 0 {
					frameAnalysis, err := p.cfg.VisionProvider.AnalyzeImage(ctx, kf.FrameData, "image/jpeg")
					if err == nil {
						sceneDescriptions = append(sceneDescriptions, fmt.Sprintf("Scene %d (%.1fs): %s", i, scene.Start, frameAnalysis.Description))
					}
				}
			}
		}
		if len(sceneDescriptions) > 0 {
			result.Summary = strings.Join(sceneDescriptions, " | ")
		}
	}

	return result, nil
}

func (p *MediaUnderstandingPipeline) UnderstandAudio(ctx context.Context, audioData []byte) (*MediaUnderstandingResult, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	result := &MediaUnderstandingResult{
		Type:     "audio",
		Metadata: map[string]any{},
	}

	if p.cfg.AudioAnalyzer == nil {
		p.cfg.AudioAnalyzer = NewAudioAnalyzer()
	}

	audioAnalysis, err := p.cfg.AudioAnalyzer.Analyze(ctx, audioData)
	if err != nil {
		return nil, fmt.Errorf("audio analysis: %w", err)
	}

	result.Details = audioAnalysis
	result.Metadata["duration"] = audioAnalysis.Duration
	result.Metadata["codec"] = audioAnalysis.Codec
	result.Metadata["sample_rate"] = audioAnalysis.SampleRate
	result.Metadata["channels"] = audioAnalysis.Channels
	result.Metadata["is_speech"] = audioAnalysis.IsSpeech
	result.Metadata["is_music"] = audioAnalysis.IsMusic
	result.Metadata["speech_confidence"] = audioAnalysis.SpeechConfidence
	result.Metadata["music_confidence"] = audioAnalysis.MusicConfidence
	result.Metadata["silence_ratio"] = audioAnalysis.Metadata["silence_ratio"]

	if audioAnalysis.IsSpeech {
		result.Summary = fmt.Sprintf("Speech audio (%.0f%% confidence)", audioAnalysis.SpeechConfidence*100)
	} else if audioAnalysis.IsMusic {
		result.Summary = fmt.Sprintf("Music audio (%.0f%% confidence)", audioAnalysis.MusicConfidence*100)
	} else {
		result.Summary = "Unknown audio type"
	}

	return result, nil
}

func (p *MediaUnderstandingPipeline) OCRImage(ctx context.Context, imageData []byte, mimeType string) (string, error) {
	if p.cfg.VisionProvider == nil {
		return "", fmt.Errorf("no vision provider configured")
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	texts, err := p.cfg.VisionProvider.OCR(ctx, imageData, mimeType)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, t := range texts {
		parts = append(parts, t.Text)
	}

	return strings.Join(parts, "\n"), nil
}

func (p *MediaUnderstandingPipeline) DescribeImageWithLLM(ctx context.Context, imageData []byte, mimeType string, client llm.Client, prompt string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no LLM client provided")
	}

	if prompt == "" {
		prompt = "Describe this image in detail. Include objects, text, people, actions, colors, and overall scene."
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	msg := llm.NewUserMessage(
		llm.TextBlock(prompt),
		llm.ImageBlockFromBase64(imageData, mimeType),
	)

	resp, err := client.Chat(ctx, []llm.Message{msg}, nil)
	if err != nil {
		return "", fmt.Errorf("LLM vision analysis: %w", err)
	}

	return resp.Content, nil
}

func ImageToDataURL(imageData []byte, mimeType string) string {
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageData))
}

func ImageFromDataURL(dataURL string) ([]byte, string, error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return nil, "", fmt.Errorf("not a data URL")
	}

	commaIdx := strings.Index(dataURL, ",")
	if commaIdx == -1 {
		return nil, "", fmt.Errorf("invalid data URL format")
	}

	header := dataURL[5:commaIdx]
	data := dataURL[commaIdx+1:]

	mimeType := ""
	if idx := strings.Index(header, ";"); idx != -1 {
		mimeType = header[:idx]
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", fmt.Errorf("decode base64: %w", err)
	}

	return decoded, mimeType, nil
}

func AnalyzeMedia(ctx context.Context, pipeline *MediaUnderstandingPipeline, data []byte, mimeType string) (*MediaUnderstandingResult, error) {
	mediaType := detectMediaType(mimeType)

	switch mediaType {
	case "image":
		return pipeline.UnderstandImage(ctx, data, mimeType)
	case "video":
		return pipeline.UnderstandVideo(ctx, data)
	case "audio":
		return pipeline.UnderstandAudio(ctx, data)
	default:
		return nil, fmt.Errorf("unsupported media type: %s", mimeType)
	}
}

func detectMediaType(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	default:
		return ""
	}
}

func AnalysisResultToJSON(result *MediaUnderstandingResult) string {
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data)
}
