package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

type AnalysisResult struct {
	Description string           `json:"description"`
	Labels      []Label          `json:"labels"`
	Objects     []DetectedObject `json:"objects"`
	Text        []DetectedText   `json:"text"`
	Faces       []DetectedFace   `json:"faces"`
	SafeSearch  SafeSearchResult `json:"safe_search"`
	ImageProps  ImageProperties  `json:"image_properties"`
	Metadata    map[string]any   `json:"metadata"`
}

type Label struct {
	Name        string  `json:"name"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`
}

type DetectedObject struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	Bounds     Bounds  `json:"bounds"`
}

type DetectedText struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Bounds     Bounds  `json:"bounds"`
	Language   string  `json:"language"`
}

type DetectedFace struct {
	Confidence float64            `json:"confidence"`
	Bounds     Bounds             `json:"bounds"`
	Emotions   map[string]float64 `json:"emotions"`
}

type Bounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type SafeSearchResult struct {
	Adult    float64 `json:"adult"`
	Violence float64 `json:"violence"`
	Medical  float64 `json:"medical"`
	Spoof    float64 `json:"spoof"`
}

type ImageProperties struct {
	DominantColors []string `json:"dominant_colors"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	Format         string   `json:"format"`
}

type VisionProvider interface {
	Name() string
	AnalyzeImage(ctx context.Context, imageData []byte, mimeType string) (*AnalysisResult, error)
	AnalyzeImageURL(ctx context.Context, imageURL string) (*AnalysisResult, error)
	OCR(ctx context.Context, imageData []byte, mimeType string) ([]DetectedText, error)
	LabelImage(ctx context.Context, imageData []byte, mimeType string) ([]Label, error)
	DetectObjects(ctx context.Context, imageData []byte, mimeType string) ([]DetectedObject, error)
}

type GoogleVisionConfig struct {
	APIKey     string
	Endpoint   string
	Features   []string
	MaxResults int
	Timeout    time.Duration
}

func DefaultGoogleVisionConfig() GoogleVisionConfig {
	return GoogleVisionConfig{
		Endpoint:   "https://vision.googleapis.com/v1",
		Features:   []string{"LABEL_DETECTION", "TEXT_DETECTION", "OBJECT_LOCALIZATION", "FACE_DETECTION", "SAFE_SEARCH_DETECTION", "IMAGE_PROPERTIES"},
		MaxResults: 10,
		Timeout:    30 * time.Second,
	}
}

type GoogleVisionProvider struct {
	cfg    GoogleVisionConfig
	client *http.Client
}

func NewGoogleVisionProvider(cfg GoogleVisionConfig) *GoogleVisionProvider {
	return &GoogleVisionProvider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *GoogleVisionProvider) Name() string {
	return "google-vision"
}

func (p *GoogleVisionProvider) AnalyzeImage(ctx context.Context, imageData []byte, mimeType string) (*AnalysisResult, error) {
	encoded := base64.StdEncoding.EncodeToString(imageData)

	requests := []map[string]any{}
	features := make([]map[string]any, 0, len(p.cfg.Features))
	for _, f := range p.cfg.Features {
		features = append(features, map[string]any{
			"type":       f,
			"maxResults": p.cfg.MaxResults,
		})
	}

	requests = append(requests, map[string]any{
		"image":    map[string]any{"content": encoded},
		"features": features,
	})

	body := map[string]any{"requests": requests}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/images:annotate?key=%s", p.cfg.Endpoint, p.cfg.APIKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google Vision API error: %s - %s", resp.Status, string(respBody))
	}

	var apiResp struct {
		Responses []struct {
			LabelAnnotations          []gcpAnnotation `json:"labelAnnotations"`
			TextAnnotations           []gcpAnnotation `json:"textAnnotations"`
			ObjectLocalizations       []gcpObjectAnn  `json:"objectLocalizations"`
			FaceAnnotations           []gcpFaceAnn    `json:"faceAnnotations"`
			SafeSearchAnnotation      gcpSafeSearch   `json:"safeSearchAnnotation"`
			ImagePropertiesAnnotation gcpImageProps   `json:"imagePropertiesAnnotation"`
			Error                     *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"responses"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(apiResp.Responses) == 0 {
		return nil, fmt.Errorf("empty response from Google Vision API")
	}

	r := apiResp.Responses[0]
	if r.Error != nil {
		return nil, fmt.Errorf("Google Vision API error: %s", r.Error.Message)
	}

	result := &AnalysisResult{Metadata: make(map[string]any)}

	for _, ann := range r.LabelAnnotations {
		result.Labels = append(result.Labels, Label{
			Name:       ann.Description,
			Confidence: float64(ann.Score),
		})
	}

	if len(result.Labels) > 0 {
		result.Description = result.Labels[0].Name
	}

	for _, ann := range r.TextAnnotations {
		dt := DetectedText{
			Text:       ann.Description,
			Confidence: float64(ann.Score),
		}
		if ann.BoundingPoly != nil && len(ann.BoundingPoly.Vertices) >= 2 {
			dt.Bounds = Bounds{
				X:      ann.BoundingPoly.Vertices[0].X,
				Y:      ann.BoundingPoly.Vertices[0].Y,
				Width:  ann.BoundingPoly.Vertices[1].X - ann.BoundingPoly.Vertices[0].X,
				Height: ann.BoundingPoly.Vertices[2].Y - ann.BoundingPoly.Vertices[0].Y,
			}
		}
		result.Text = append(result.Text, dt)
	}

	for _, obj := range r.ObjectLocalizations {
		detObj := DetectedObject{
			Name:       obj.Name,
			Confidence: float64(obj.Score),
		}
		if obj.BoundingPoly != nil && len(obj.BoundingPoly.Vertices) >= 2 {
			detObj.Bounds = Bounds{
				X:      obj.BoundingPoly.Vertices[0].X,
				Y:      obj.BoundingPoly.Vertices[0].Y,
				Width:  obj.BoundingPoly.Vertices[1].X - obj.BoundingPoly.Vertices[0].X,
				Height: obj.BoundingPoly.Vertices[2].Y - obj.BoundingPoly.Vertices[0].Y,
			}
		}
		result.Objects = append(result.Objects, detObj)
	}

	for _, face := range r.FaceAnnotations {
		emotions := make(map[string]float64)
		emotions["joy"] = likelihoodToFloat(face.JoyLikelihood)
		emotions["sorrow"] = likelihoodToFloat(face.SorrowLikelihood)
		emotions["anger"] = likelihoodToFloat(face.AngerLikelihood)
		emotions["surprise"] = likelihoodToFloat(face.SurpriseLikelihood)

		detFace := DetectedFace{
			Confidence: float64(face.DetectionConfidence),
			Emotions:   emotions,
		}
		if face.BoundingPoly != nil && len(face.BoundingPoly.Vertices) >= 2 {
			detFace.Bounds = Bounds{
				X:      face.BoundingPoly.Vertices[0].X,
				Y:      face.BoundingPoly.Vertices[0].Y,
				Width:  face.BoundingPoly.Vertices[1].X - face.BoundingPoly.Vertices[0].X,
				Height: face.BoundingPoly.Vertices[2].Y - face.BoundingPoly.Vertices[0].Y,
			}
		}
		result.Faces = append(result.Faces, detFace)
	}

	result.SafeSearch = SafeSearchResult{
		Adult:    likelihoodToFloat(r.SafeSearchAnnotation.Adult),
		Violence: likelihoodToFloat(r.SafeSearchAnnotation.Violence),
		Medical:  likelihoodToFloat(r.SafeSearchAnnotation.Medical),
		Spoof:    likelihoodToFloat(r.SafeSearchAnnotation.Spoof),
	}

	if len(r.ImagePropertiesAnnotation.DominantColors.Colors) > 0 {
		for _, c := range r.ImagePropertiesAnnotation.DominantColors.Colors {
			result.ImageProps.DominantColors = append(result.ImageProps.DominantColors,
				fmt.Sprintf("#%02X%02X%02X", c.Color.Red, c.Color.Green, c.Color.Blue))
		}
	}

	return result, nil
}

func (p *GoogleVisionProvider) AnalyzeImageURL(ctx context.Context, imageURL string) (*AnalysisResult, error) {
	resp, err := p.client.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	return p.AnalyzeImage(ctx, data, mimeType)
}

func (p *GoogleVisionProvider) OCR(ctx context.Context, imageData []byte, mimeType string) ([]DetectedText, error) {
	result, err := p.AnalyzeImage(ctx, imageData, mimeType)
	if err != nil {
		return nil, err
	}
	return result.Text, nil
}

func (p *GoogleVisionProvider) LabelImage(ctx context.Context, imageData []byte, mimeType string) ([]Label, error) {
	result, err := p.AnalyzeImage(ctx, imageData, mimeType)
	if err != nil {
		return nil, err
	}
	return result.Labels, nil
}

func (p *GoogleVisionProvider) DetectObjects(ctx context.Context, imageData []byte, mimeType string) ([]DetectedObject, error) {
	result, err := p.AnalyzeImage(ctx, imageData, mimeType)
	if err != nil {
		return nil, err
	}
	return result.Objects, nil
}

type gcpAnnotation struct {
	Mid          string  `json:"mid"`
	Description  string  `json:"description"`
	Score        float32 `json:"score"`
	Confidence   float32 `json:"confidence"`
	Topicality   float32 `json:"topicality"`
	BoundingPoly *struct {
		Vertices []struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"vertices"`
	} `json:"boundingPoly"`
}

type gcpObjectAnn struct {
	Name         string  `json:"name"`
	Score        float32 `json:"score"`
	BoundingPoly *struct {
		Vertices []struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"vertices"`
	} `json:"boundingPoly"`
}

type gcpFaceAnn struct {
	BoundingPoly *struct {
		Vertices []struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"vertices"`
	} `json:"boundingPoly"`
	DetectionConfidence   float32 `json:"detectionConfidence"`
	LandmarkingConfidence float32 `json:"landmarkingConfidence"`
	JoyLikelihood         string  `json:"joyLikelihood"`
	SorrowLikelihood      string  `json:"sorrowLikelihood"`
	AngerLikelihood       string  `json:"angerLikelihood"`
	SurpriseLikelihood    string  `json:"surpriseLikelihood"`
}

type gcpSafeSearch struct {
	Adult    string `json:"adult"`
	Spoof    string `json:"spoof"`
	Medical  string `json:"medical"`
	Violence string `json:"violence"`
	Racy     string `json:"racy"`
}

type gcpImageProps struct {
	DominantColors struct {
		Colors []struct {
			Color struct {
				Red   int `json:"red"`
				Green int `json:"green"`
				Blue  int `json:"blue"`
			} `json:"color"`
			Score float32 `json:"score"`
		} `json:"colors"`
	} `json:"dominantColors"`
}

func likelihoodToFloat(l string) float64 {
	switch strings.ToLower(l) {
	case "very_likely":
		return 0.9
	case "likely":
		return 0.7
	case "possible":
		return 0.5
	case "unlikely":
		return 0.3
	case "very_unlikely":
		return 0.1
	default:
		return 0
	}
}

type LLMVisionConfig struct {
	Client      LLMVisionClient
	Prompt      string
	MaxTokens   int
	Temperature float64
}

func DefaultLLMVisionConfig() LLMVisionConfig {
	return LLMVisionConfig{
		Prompt:      "Describe this image in detail. Include objects, text, people, actions, colors, and overall scene.",
		MaxTokens:   1024,
		Temperature: 0.3,
	}
}

type LLMVisionClient interface {
	AnalyzeImageWithPrompt(ctx context.Context, imageData []byte, mimeType string, prompt string) (string, error)
}

type LLMVisionProvider struct {
	cfg LLMVisionConfig
}

func NewLLMVisionProvider(cfg LLMVisionConfig) *LLMVisionProvider {
	return &LLMVisionProvider{cfg: cfg}
}

func (p *LLMVisionProvider) Name() string {
	return "llm-vision"
}

func (p *LLMVisionProvider) AnalyzeImage(ctx context.Context, imageData []byte, mimeType string) (*AnalysisResult, error) {
	prompt := p.cfg.Prompt
	if prompt == "" {
		prompt = "Describe this image in detail."
	}

	description, err := p.cfg.Client.AnalyzeImageWithPrompt(ctx, imageData, mimeType, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM vision analysis: %w", err)
	}

	return &AnalysisResult{
		Description: description,
		Metadata:    map[string]any{"provider": "llm-vision", "prompt": prompt},
	}, nil
}

func (p *LLMVisionProvider) AnalyzeImageURL(ctx context.Context, imageURL string) (*AnalysisResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	return p.AnalyzeImage(ctx, data, mimeType)
}

func (p *LLMVisionProvider) OCR(ctx context.Context, imageData []byte, mimeType string) ([]DetectedText, error) {
	prompt := "Extract all text from this image. Return only the text content, preserving line breaks."
	description, err := p.cfg.Client.AnalyzeImageWithPrompt(ctx, imageData, mimeType, prompt)
	if err != nil {
		return nil, err
	}

	return []DetectedText{
		{Text: description, Confidence: 1.0},
	}, nil
}

func (p *LLMVisionProvider) LabelImage(ctx context.Context, imageData []byte, mimeType string) ([]Label, error) {
	prompt := "List the main objects and concepts in this image as a comma-separated list. Format each as 'name:confidence' where confidence is 0.0-1.0."
	description, err := p.cfg.Client.AnalyzeImageWithPrompt(ctx, imageData, mimeType, prompt)
	if err != nil {
		return nil, err
	}

	var labels []Label
	lines := strings.Split(description, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		confidence := 0.8
		if len(parts) == 2 {
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &confidence)
		}
		labels = append(labels, Label{Name: name, Confidence: confidence})
	}

	return labels, nil
}

func (p *LLMVisionProvider) DetectObjects(ctx context.Context, imageData []byte, mimeType string) ([]DetectedObject, error) {
	prompt := "List all visible objects in this image. For each, provide name and approximate bounding box as x,y,width,height."
	_, err := p.cfg.Client.AnalyzeImageWithPrompt(ctx, imageData, mimeType, prompt)
	if err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("object detection not supported via LLM vision")
}

func AnalyzeImageFile(ctx context.Context, provider VisionProvider, path string) (*AnalysisResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read image file: %w", err)
	}

	mimeType := mimeTypeFromPath(path)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	return provider.AnalyzeImage(ctx, data, mimeType)
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

func UploadImageForAnalysis(ctx context.Context, endpoint string, apiKey string, imageData []byte, mimeType string) (*AnalysisResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("image", "upload")
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	part.Write(imageData)

	if mimeType != "" {
		writer.WriteField("mime_type", mimeType)
	}

	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed: %s - %s", resp.Status, string(body))
	}

	var result AnalysisResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}
