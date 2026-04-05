package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/llm"
)

type imageMatchResult struct {
	Found        bool             `json:"found"`
	Score        float64          `json:"score"`
	Threshold    float64          `json:"threshold"`
	X            int              `json:"x,omitempty"`
	Y            int              `json:"y,omitempty"`
	Width        int              `json:"width,omitempty"`
	Height       int              `json:"height,omitempty"`
	CenterX      int              `json:"center_x,omitempty"`
	CenterY      int              `json:"center_y,omitempty"`
	SourcePath   string           `json:"source_path,omitempty"`
	TemplatePath string           `json:"template_path,omitempty"`
	SearchArea   *imageSearchArea `json:"search_area,omitempty"`
	Meta         map[string]any   `json:"meta,omitempty"`
}

type imageSearchArea struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type grayImage struct {
	Width  int
	Height int
	Pix    []uint8
}

type templateMatch struct {
	X     int
	Y     int
	Score float64
}

type ocrResult struct {
	Text       string   `json:"text"`
	Lines      []string `json:"lines,omitempty"`
	SourcePath string   `json:"source_path,omitempty"`
	Engine     string   `json:"engine,omitempty"`
	Lang       string   `json:"lang,omitempty"`
}

type ocrWordBox struct {
	Text       string
	Normalized string
	Confidence float64
	X          int
	Y          int
	Width      int
	Height     int
	CenterX    int
	CenterY    int
	Key        string
	WordNum    int
}

type ocrLineBox struct {
	Text       string
	Normalized string
	Confidence float64
	X          int
	Y          int
	Width      int
	Height     int
	CenterX    int
	CenterY    int
	Words      []ocrWordBox
}

type ocrTextMatchResult struct {
	Found      bool           `json:"found"`
	Query      string         `json:"query"`
	Mode       string         `json:"mode"`
	IgnoreCase bool           `json:"ignore_case"`
	Scope      string         `json:"scope,omitempty"`
	Text       string         `json:"text,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	X          int            `json:"x,omitempty"`
	Y          int            `json:"y,omitempty"`
	Width      int            `json:"width,omitempty"`
	Height     int            `json:"height,omitempty"`
	CenterX    int            `json:"center_x,omitempty"`
	CenterY    int            `json:"center_y,omitempty"`
	SourcePath string         `json:"source_path,omitempty"`
	Engine     string         `json:"engine,omitempty"`
	Lang       string         `json:"lang,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}

func DesktopMatchImageTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_match_image", opts, true); err != nil {
		return "", err
	}
	result, err := runDesktopImageMatch(ctx, input, opts)
	if err != nil {
		return "", err
	}
	return marshalImageMatchResult(result)
}

func DesktopClickImageTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_click_image", opts, false); err != nil {
		return "", err
	}
	result, err := runDesktopImageMatch(ctx, input, opts)
	if err != nil {
		return "", err
	}
	if !result.Found {
		return "", fmt.Errorf("image not found (score %.4f below threshold %.4f)", result.Score, result.Threshold)
	}
	clickInput := map[string]any{
		"x": result.CenterX + intNumberValue(input["offset_x"]),
		"y": result.CenterY + intNumberValue(input["offset_y"]),
	}
	if button, _ := input["button"].(string); strings.TrimSpace(button) != "" {
		clickInput["button"] = strings.TrimSpace(button)
	}
	doubleClick, _ := input["double"].(bool)
	if doubleClick {
		if intervalMS, ok := numberInput(input["interval_ms"]); ok && intervalMS > 0 {
			clickInput["interval_ms"] = intervalMS
		}
		if _, err := DesktopDoubleClickTool(ctx, clickInput, opts); err != nil {
			return "", err
		}
		result.Meta = mergeMeta(result.Meta, map[string]any{"action": "double_click"})
	} else {
		if _, err := DesktopClickTool(ctx, clickInput, opts); err != nil {
			return "", err
		}
		result.Meta = mergeMeta(result.Meta, map[string]any{"action": "click"})
	}
	result.Meta = mergeMeta(result.Meta, map[string]any{
		"clicked_x": clickInput["x"],
		"clicked_y": clickInput["y"],
	})
	return marshalImageMatchResult(result)
}

func DesktopWaitImageTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_wait_image", opts, true); err != nil {
		return "", err
	}
	timeoutMS, ok := numberInput(input["timeout_ms"])
	if !ok || timeoutMS <= 0 {
		timeoutMS = 10000
	}
	intervalMS, ok := numberInput(input["interval_ms"])
	if !ok || intervalMS <= 0 {
		intervalMS = 500
	}
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	attempts := 0
	var last imageMatchResult
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		attempts++
		result, err := runDesktopImageMatch(ctx, input, opts)
		if err != nil {
			return "", err
		}
		last = result
		if result.Found {
			result.Meta = mergeMeta(result.Meta, map[string]any{
				"attempts":   attempts,
				"timeout_ms": timeoutMS,
			})
			return marshalImageMatchResult(result)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("image did not appear within %dms (best score %.4f below threshold %.4f)", timeoutMS, last.Score, last.Threshold)
		}
		timer := time.NewTimer(time.Duration(intervalMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func DesktopOCRTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_ocr", opts, true); err != nil {
		return "", err
	}
	result, err := runDesktopOCR(ctx, input, opts)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopVerifyTextTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_verify_text", opts, true); err != nil {
		return "", err
	}
	expected, _ := input["expected"].(string)
	if strings.TrimSpace(expected) == "" {
		return "", fmt.Errorf("expected is required")
	}
	mode, _ := input["mode"].(string)
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = "contains"
	}
	if mode != "contains" && mode != "exact" && mode != "regex" {
		return "", fmt.Errorf("mode must be contains, exact, or regex")
	}
	ignoreCase := true
	if value, ok := input["ignore_case"].(bool); ok {
		ignoreCase = value
	}
	result, err := runDesktopOCR(ctx, input, opts)
	if err != nil {
		return "", err
	}
	if !verifyOCRText(result.Text, expected, mode, ignoreCase) {
		snippet := result.Text
		if len(snippet) > 240 {
			snippet = snippet[:240] + "..."
		}
		return "", fmt.Errorf("expected text not found via OCR (mode=%s): %s", mode, snippet)
	}
	response := map[string]any{
		"matched":     true,
		"mode":        mode,
		"expected":    expected,
		"ignore_case": ignoreCase,
		"ocr":         result,
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopFindTextTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_find_text", opts, true); err != nil {
		return "", err
	}
	query := strings.TrimSpace(stringValue(input["text"]))
	if query == "" {
		return "", fmt.Errorf("text is required")
	}
	result, err := runDesktopTextMatch(ctx, input, opts)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopClickTextTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_click_text", opts, false); err != nil {
		return "", err
	}
	result, err := runDesktopTextMatch(ctx, input, opts)
	if err != nil {
		return "", err
	}
	if !result.Found {
		return "", fmt.Errorf("text not found: %s", strings.TrimSpace(result.Query))
	}
	clickInput := map[string]any{
		"x": result.CenterX + intNumberValue(input["offset_x"]),
		"y": result.CenterY + intNumberValue(input["offset_y"]),
	}
	if button, _ := input["button"].(string); strings.TrimSpace(button) != "" {
		clickInput["button"] = strings.TrimSpace(button)
	}
	doubleClick, _ := input["double"].(bool)
	if doubleClick {
		if intervalMS, ok := numberInput(input["interval_ms"]); ok && intervalMS > 0 {
			clickInput["interval_ms"] = intervalMS
		}
		if _, err := DesktopDoubleClickTool(ctx, clickInput, opts); err != nil {
			return "", err
		}
		result.Meta = mergeMeta(result.Meta, map[string]any{"action": "double_click"})
	} else {
		if _, err := DesktopClickTool(ctx, clickInput, opts); err != nil {
			return "", err
		}
		result.Meta = mergeMeta(result.Meta, map[string]any{"action": "click"})
	}
	result.Meta = mergeMeta(result.Meta, map[string]any{
		"clicked_x": clickInput["x"],
		"clicked_y": clickInput["y"],
	})
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DesktopWaitTextTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	if err := ensureDesktopAllowed("desktop_wait_text", opts, true); err != nil {
		return "", err
	}
	query := strings.TrimSpace(stringValue(input["text"]))
	if query == "" {
		return "", fmt.Errorf("text is required")
	}
	timeoutMS, ok := numberInput(input["timeout_ms"])
	if !ok || timeoutMS <= 0 {
		timeoutMS = 10000
	}
	intervalMS, ok := numberInput(input["interval_ms"])
	if !ok || intervalMS <= 0 {
		intervalMS = 500
	}
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	attempts := 0
	var last ocrTextMatchResult
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		attempts++
		result, err := runDesktopTextMatch(ctx, input, opts)
		if err != nil {
			return "", err
		}
		last = result
		if result.Found {
			result.Meta = mergeMeta(result.Meta, map[string]any{
				"attempts":   attempts,
				"timeout_ms": timeoutMS,
			})
			payload, err := json.Marshal(result)
			if err != nil {
				return "", err
			}
			return string(payload), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("text did not appear within %dms: %s", timeoutMS, strings.TrimSpace(last.Query))
		}
		timer := time.NewTimer(time.Duration(intervalMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func runDesktopImageMatch(ctx context.Context, input map[string]any, opts BuiltinOptions) (imageMatchResult, error) {
	sourcePath, cleanup, err := resolveDesktopVisionSource(ctx, input, opts)
	if err != nil {
		return imageMatchResult{}, err
	}
	defer cleanup()

	templatePath, err := resolveDesktopVisionTemplate(input, opts)
	if err != nil {
		return imageMatchResult{}, err
	}

	sourceImage, err := loadImageFile(sourcePath)
	if err != nil {
		return imageMatchResult{}, err
	}
	templateImage, err := loadImageFile(templatePath)
	if err != nil {
		return imageMatchResult{}, err
	}

	sourceBounds := normalizeBounds(sourceImage.Bounds())
	templateBounds := normalizeBounds(templateImage.Bounds())
	searchRect, err := resolveSearchRect(input, sourceBounds, templateBounds)
	if err != nil {
		return imageMatchResult{}, err
	}
	step := resolveSearchStep(input, searchRect, templateBounds)
	match, err := locateTemplate(sourceImage, templateImage, searchRect, step)
	if err != nil {
		return imageMatchResult{}, err
	}

	threshold := numberInputFloat(input["threshold"], 0.92)
	found := match.Score >= threshold
	return imageMatchResult{
		Found:        found,
		Score:        round4(match.Score),
		Threshold:    round4(threshold),
		X:            match.X,
		Y:            match.Y,
		Width:        templateBounds.Dx(),
		Height:       templateBounds.Dy(),
		CenterX:      match.X + templateBounds.Dx()/2,
		CenterY:      match.Y + templateBounds.Dy()/2,
		SourcePath:   sourcePath,
		TemplatePath: templatePath,
		SearchArea: &imageSearchArea{
			X:      searchRect.Min.X,
			Y:      searchRect.Min.Y,
			Width:  searchRect.Dx(),
			Height: searchRect.Dy(),
		},
		Meta: map[string]any{
			"step": step,
		},
	}, nil
}

func runDesktopOCR(ctx context.Context, input map[string]any, opts BuiltinOptions) (ocrResult, error) {
	sourcePath, cleanup, err := resolveDesktopVisionSource(ctx, input, opts)
	if err != nil {
		return ocrResult{}, err
	}
	defer cleanup()

	lang := strings.TrimSpace(stringValue(input["lang"]))
	text, err := runTesseractOCR(ctx, sourcePath, lang, intNumberValue(input["psm"]), intNumberValue(input["oem"]))
	if err != nil {
		return ocrResult{}, err
	}
	result := ocrResult{
		Text:       normalizeOCRText(text),
		SourcePath: sourcePath,
		Engine:     "tesseract",
		Lang:       lang,
	}
	result.Lines = splitOCRLines(result.Text)
	return result, nil
}

func runDesktopTextMatch(ctx context.Context, input map[string]any, opts BuiltinOptions) (ocrTextMatchResult, error) {
	sourcePath, cleanup, err := resolveDesktopVisionSource(ctx, input, opts)
	if err != nil {
		return ocrTextMatchResult{}, err
	}
	defer cleanup()

	query := strings.TrimSpace(stringValue(input["text"]))
	mode := strings.TrimSpace(strings.ToLower(stringValue(input["mode"])))
	if mode == "" {
		mode = "contains"
	}
	if mode != "contains" && mode != "exact" && mode != "regex" {
		return ocrTextMatchResult{}, fmt.Errorf("mode must be contains, exact, or regex")
	}
	ignoreCase := true
	if value, ok := input["ignore_case"].(bool); ok {
		ignoreCase = value
	}
	occurrence := intNumberValue(input["occurrence"])
	if occurrence <= 0 {
		occurrence = 1
	}
	minConfidence := numberInputFloat(input["min_confidence"], 30)
	lang := strings.TrimSpace(stringValue(input["lang"]))

	tsv, err := runTesseractOCRTSV(ctx, sourcePath, lang, intNumberValue(input["psm"]), intNumberValue(input["oem"]))
	if err != nil {
		return ocrTextMatchResult{}, err
	}
	words, err := parseTesseractTSV(tsv)
	if err != nil {
		return ocrTextMatchResult{}, err
	}
	lines := buildOCRLineBoxes(words, minConfidence)
	result := findOCRTextMatch(lines, words, query, mode, ignoreCase, occurrence, minConfidence)
	result.Query = query
	result.Mode = mode
	result.IgnoreCase = ignoreCase
	result.SourcePath = sourcePath
	result.Engine = "tesseract"
	result.Lang = lang
	result.Meta = mergeMeta(result.Meta, map[string]any{
		"occurrence":     occurrence,
		"min_confidence": round4(minConfidence),
		"line_count":     len(lines),
		"word_count":     len(words),
	})
	return result, nil
}

func resolveDesktopVisionSource(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, func(), error) {
	if path, _ := input["path"].(string); strings.TrimSpace(path) != "" {
		resolved := resolvePath(strings.TrimSpace(path), opts.WorkingDir)
		if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
			return "", nil, err
		}
		return resolved, func() {}, nil
	}
	tempFile, err := os.CreateTemp("", "anyclaw-desktop-*.png")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp screenshot file: %w", err)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()
	if _, err := captureDesktopScreenshotToPath(ctx, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", nil, err
	}
	return tempPath, func() { _ = os.Remove(tempPath) }, nil
}

func resolveDesktopVisionTemplate(input map[string]any, opts BuiltinOptions) (string, error) {
	path, _ := input["template_path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("template_path is required")
	}
	resolved := resolvePath(strings.TrimSpace(path), opts.WorkingDir)
	if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	return resolved, nil
}

func loadImageFile(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", path, err)
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image %s: %w", path, err)
	}
	return img, nil
}

func resolveSearchRect(input map[string]any, source image.Rectangle, template image.Rectangle) (image.Rectangle, error) {
	x, hasX := numberInput(input["search_x"])
	y, hasY := numberInput(input["search_y"])
	width, hasWidth := numberInput(input["search_width"])
	height, hasHeight := numberInput(input["search_height"])

	rect := source
	if hasX || hasY || hasWidth || hasHeight {
		if !hasWidth || !hasHeight {
			return image.Rectangle{}, fmt.Errorf("search_width and search_height are required when using search bounds")
		}
		if !hasX {
			x = 0
		}
		if !hasY {
			y = 0
		}
		if width <= 0 || height <= 0 {
			return image.Rectangle{}, fmt.Errorf("search_width and search_height must be positive")
		}
		rect = image.Rect(x, y, x+width, y+height).Intersect(source)
	}
	if rect.Dx() < template.Dx() || rect.Dy() < template.Dy() {
		return image.Rectangle{}, fmt.Errorf("search area is smaller than template")
	}
	return rect, nil
}

func resolveSearchStep(input map[string]any, searchRect image.Rectangle, template image.Rectangle) int {
	if step, ok := numberInput(input["step"]); ok && step > 0 {
		return step
	}
	positions := (searchRect.Dx() - template.Dx() + 1) * (searchRect.Dy() - template.Dy() + 1)
	switch {
	case positions > 2_000_000:
		return 4
	case positions > 600_000:
		return 2
	default:
		return 1
	}
}

func locateTemplate(source image.Image, template image.Image, searchRect image.Rectangle, step int) (templateMatch, error) {
	srcGray := toGrayImage(source)
	tplGray := toGrayImage(template)
	if tplGray.Width <= 0 || tplGray.Height <= 0 {
		return templateMatch{}, fmt.Errorf("template image is empty")
	}
	maxX := searchRect.Max.X - tplGray.Width
	maxY := searchRect.Max.Y - tplGray.Height
	if maxX < searchRect.Min.X || maxY < searchRect.Min.Y {
		return templateMatch{}, fmt.Errorf("template does not fit inside search area")
	}

	coarseSamples := samplePointsForSize(tplGray.Width, tplGray.Height, 6)
	best := templateMatch{Score: -1}
	for y := searchRect.Min.Y; y <= maxY; y += step {
		for x := searchRect.Min.X; x <= maxX; x += step {
			score := similarityScore(srcGray, tplGray, x, y, coarseSamples)
			if score > best.Score {
				best = templateMatch{X: x, Y: y, Score: score}
			}
		}
	}
	if best.Score < 0 {
		return templateMatch{}, fmt.Errorf("no search positions available")
	}

	refineMinX := maxInt(searchRect.Min.X, best.X-step)
	refineMaxX := minInt(maxX, best.X+step)
	refineMinY := maxInt(searchRect.Min.Y, best.Y-step)
	refineMaxY := minInt(maxY, best.Y+step)
	fineSamples := samplePointsForSize(tplGray.Width, tplGray.Height, 14)
	for y := refineMinY; y <= refineMaxY; y++ {
		for x := refineMinX; x <= refineMaxX; x++ {
			score := similarityScore(srcGray, tplGray, x, y, fineSamples)
			if score > best.Score {
				best = templateMatch{X: x, Y: y, Score: score}
			}
		}
	}
	return best, nil
}

func toGrayImage(img image.Image) grayImage {
	bounds := normalizeBounds(img.Bounds())
	width := bounds.Dx()
	height := bounds.Dy()
	pix := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			luma := ((299*r + 587*g + 114*b + 500) / 1000) >> 8
			pix[y*width+x] = uint8(luma)
		}
	}
	return grayImage{Width: width, Height: height, Pix: pix}
}

func samplePointsForSize(width int, height int, grid int) []image.Point {
	if width <= 0 || height <= 0 {
		return nil
	}
	if grid <= 1 {
		return []image.Point{{0, 0}}
	}
	points := make([]image.Point, 0, grid*grid)
	seen := map[image.Point]bool{}
	for gy := 0; gy < grid; gy++ {
		y := scalePoint(gy, grid, height)
		for gx := 0; gx < grid; gx++ {
			x := scalePoint(gx, grid, width)
			point := image.Pt(x, y)
			if !seen[point] {
				seen[point] = true
				points = append(points, point)
			}
		}
	}
	return points
}

func scalePoint(index int, grid int, size int) int {
	if size <= 1 || grid <= 1 {
		return 0
	}
	if index >= grid-1 {
		return size - 1
	}
	value := int(math.Round(float64(index) * float64(size-1) / float64(grid-1)))
	if value < 0 {
		return 0
	}
	if value >= size {
		return size - 1
	}
	return value
}

func similarityScore(source grayImage, template grayImage, offsetX int, offsetY int, samples []image.Point) float64 {
	if len(samples) == 0 {
		return 0
	}
	var total float64
	for _, point := range samples {
		srcValue := source.Pix[(offsetY+point.Y)*source.Width+(offsetX+point.X)]
		tplValue := template.Pix[point.Y*template.Width+point.X]
		diff := math.Abs(float64(srcValue) - float64(tplValue))
		total += diff / 255.0
	}
	score := 1 - total/float64(len(samples))
	if score < 0 {
		return 0
	}
	return score
}

func normalizeBounds(bounds image.Rectangle) image.Rectangle {
	return image.Rect(0, 0, bounds.Dx(), bounds.Dy())
}

func marshalImageMatchResult(result imageMatchResult) (string, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func numberInputFloat(value any, fallback float64) float64 {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return v
		}
	case float32:
		if v > 0 {
			return float64(v)
		}
	case int:
		if v > 0 {
			return float64(v)
		}
	case int64:
		if v > 0 {
			return float64(v)
		}
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback
		}
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func intNumberValue(value any) int {
	n, ok := numberInput(value)
	if !ok {
		return 0
	}
	return n
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func mergeMeta(dst map[string]any, src map[string]any) map[string]any {
	if dst == nil && src == nil {
		return nil
	}
	if dst == nil {
		dst = map[string]any{}
	}
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func runTesseractOCR(ctx context.Context, path string, lang string, psm int, oem int) (string, error) {
	return runTesseractOutput(ctx, path, lang, psm, oem, "")
}

func runTesseractOCRTSV(ctx context.Context, path string, lang string, psm int, oem int) (string, error) {
	return runTesseractOutput(ctx, path, lang, psm, oem, "tsv")
}

func runTesseractOutput(ctx context.Context, path string, lang string, psm int, oem int, format string) (string, error) {
	exe, err := exec.LookPath("tesseract")
	if err != nil {
		return "", fmt.Errorf("desktop_ocr requires Tesseract OCR in PATH")
	}
	args := []string{path, "stdout"}
	if psm <= 0 {
		psm = 6
	}
	args = append(args, "--psm", strconv.Itoa(psm))
	if oem > 0 {
		args = append(args, "--oem", strconv.Itoa(oem))
	}
	if strings.TrimSpace(lang) != "" {
		args = append(args, "-l", strings.TrimSpace(lang))
	}
	if strings.TrimSpace(format) != "" {
		args = append(args, strings.TrimSpace(format))
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tesseract OCR failed: %w - %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func normalizeOCRText(text string) string {
	text = strings.ReplaceAll(text, "\u000c", "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := splitOCRLines(text)
	return strings.Join(lines, "\n")
}

func normalizeOCRSearchText(text string) string {
	text = strings.TrimSpace(strings.ToLower(normalizeOCRText(text)))
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func splitOCRLines(text string) []string {
	rawLines := strings.Split(strings.TrimSpace(text), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func verifyOCRText(actual string, expected string, mode string, ignoreCase bool) bool {
	actual = normalizeOCRText(actual)
	expected = normalizeOCRText(expected)
	if ignoreCase {
		actual = strings.ToLower(actual)
		expected = strings.ToLower(expected)
	}
	switch mode {
	case "exact":
		return actual == expected
	case "regex":
		pattern := expected
		if ignoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(actual)
	default:
		return strings.Contains(actual, expected)
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func parseTesseractTSV(tsv string) ([]ocrWordBox, error) {
	lines := strings.Split(strings.ReplaceAll(tsv, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty tesseract TSV output")
	}
	header := strings.Split(lines[0], "\t")
	indexes := map[string]int{}
	for i, item := range header {
		indexes[strings.TrimSpace(strings.ToLower(item))] = i
	}
	required := []string{"level", "block_num", "par_num", "line_num", "word_num", "left", "top", "width", "height", "conf", "text"}
	for _, key := range required {
		if _, ok := indexes[key]; !ok {
			return nil, fmt.Errorf("invalid tesseract TSV header: missing %s", key)
		}
	}
	words := make([]ocrWordBox, 0)
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < len(header) {
			continue
		}
		if fieldValue(fields, indexes, "level") != "5" {
			continue
		}
		text := strings.TrimSpace(fieldValue(fields, indexes, "text"))
		if text == "" {
			continue
		}
		x, okX := numberInput(fieldValue(fields, indexes, "left"))
		y, okY := numberInput(fieldValue(fields, indexes, "top"))
		width, okW := numberInput(fieldValue(fields, indexes, "width"))
		height, okH := numberInput(fieldValue(fields, indexes, "height"))
		wordNum, _ := numberInput(fieldValue(fields, indexes, "word_num"))
		if !okX || !okY || !okW || !okH || width <= 0 || height <= 0 {
			continue
		}
		words = append(words, ocrWordBox{
			Text:       text,
			Normalized: normalizeOCRSearchText(text),
			Confidence: numberInputFloat(fieldValue(fields, indexes, "conf"), 0),
			X:          x,
			Y:          y,
			Width:      width,
			Height:     height,
			CenterX:    x + width/2,
			CenterY:    y + height/2,
			Key: strings.Join([]string{
				fieldValue(fields, indexes, "block_num"),
				fieldValue(fields, indexes, "par_num"),
				fieldValue(fields, indexes, "line_num"),
			}, ":"),
			WordNum: wordNum,
		})
	}
	return words, nil
}

func fieldValue(fields []string, indexes map[string]int, key string) string {
	idx, ok := indexes[key]
	if !ok || idx < 0 || idx >= len(fields) {
		return ""
	}
	return fields[idx]
}

func buildOCRLineBoxes(words []ocrWordBox, minConfidence float64) []ocrLineBox {
	grouped := map[string][]ocrWordBox{}
	for _, word := range words {
		if word.Confidence < minConfidence {
			continue
		}
		grouped[word.Key] = append(grouped[word.Key], word)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]ocrLineBox, 0, len(keys))
	for _, key := range keys {
		items := append([]ocrWordBox(nil), grouped[key]...)
		sort.Slice(items, func(i, j int) bool {
			if items[i].Y == items[j].Y {
				return items[i].WordNum < items[j].WordNum
			}
			return items[i].Y < items[j].Y
		})
		texts := make([]string, 0, len(items))
		minX, minY := 0, 0
		maxX, maxY := 0, 0
		var totalConfidence float64
		for i, word := range items {
			texts = append(texts, word.Text)
			totalConfidence += word.Confidence
			if i == 0 || word.X < minX {
				minX = word.X
			}
			if i == 0 || word.Y < minY {
				minY = word.Y
			}
			if i == 0 || word.X+word.Width > maxX {
				maxX = word.X + word.Width
			}
			if i == 0 || word.Y+word.Height > maxY {
				maxY = word.Y + word.Height
			}
		}
		text := strings.Join(texts, " ")
		lines = append(lines, ocrLineBox{
			Text:       text,
			Normalized: normalizeOCRSearchText(text),
			Confidence: round4(totalConfidence / float64(len(items))),
			X:          minX,
			Y:          minY,
			Width:      maxX - minX,
			Height:     maxY - minY,
			CenterX:    minX + (maxX-minX)/2,
			CenterY:    minY + (maxY-minY)/2,
			Words:      items,
		})
	}
	return lines
}

func findOCRTextMatch(lines []ocrLineBox, words []ocrWordBox, query string, mode string, ignoreCase bool, occurrence int, minConfidence float64) ocrTextMatchResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return ocrTextMatchResult{}
	}
	matchCount := 0
	for _, line := range lines {
		if matchesOCRText(line.Text, query, mode, ignoreCase) {
			matchCount++
			if matchCount == occurrence {
				return ocrTextMatchResult{
					Found:      true,
					Query:      query,
					Mode:       mode,
					IgnoreCase: ignoreCase,
					Scope:      "line",
					Text:       line.Text,
					Confidence: line.Confidence,
					X:          line.X,
					Y:          line.Y,
					Width:      line.Width,
					Height:     line.Height,
					CenterX:    line.CenterX,
					CenterY:    line.CenterY,
				}
			}
		}
	}
	for _, word := range words {
		if word.Confidence < minConfidence {
			continue
		}
		if matchesOCRText(word.Text, query, mode, ignoreCase) {
			matchCount++
			if matchCount == occurrence {
				return ocrTextMatchResult{
					Found:      true,
					Query:      query,
					Mode:       mode,
					IgnoreCase: ignoreCase,
					Scope:      "word",
					Text:       word.Text,
					Confidence: round4(word.Confidence),
					X:          word.X,
					Y:          word.Y,
					Width:      word.Width,
					Height:     word.Height,
					CenterX:    word.CenterX,
					CenterY:    word.CenterY,
				}
			}
		}
	}
	return ocrTextMatchResult{
		Found:      false,
		Query:      query,
		Mode:       mode,
		IgnoreCase: ignoreCase,
	}
}

func matchesOCRText(actual string, expected string, mode string, ignoreCase bool) bool {
	return verifyOCRText(actual, expected, mode, ignoreCase)
}

func ImageAnalyzeTool(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	path, _ := input["path"].(string)
	url, _ := input["url"].(string)
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(path) == "" && strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("image_analyze requires either path or url")
	}

	var imageData []byte
	var mimeType string
	var err error

	if strings.TrimSpace(path) != "" {
		imageData, err = os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			return "", fmt.Errorf("read image: %w", err)
		}
		mimeType = mimeTypeFromPath(path)
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
	} else {
		resp, err := http.Get(strings.TrimSpace(url))
		if err != nil {
			return "", fmt.Errorf("fetch image: %w", err)
		}
		defer resp.Body.Close()
		imageData, err = io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
		if err != nil {
			return "", fmt.Errorf("read image: %w", err)
		}
		mimeType = resp.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "image/jpeg"
		}
	}

	if prompt == "" {
		prompt = "Describe this image in detail. Include objects, text, people, actions, colors, and overall scene."
	}

	client := opts.LLMClient
	if client == nil {
		return "", fmt.Errorf("image_analyze requires an LLM client with vision capabilities (use gpt-4o, claude-3, etc.)")
	}

	msg := llm.NewUserMessage(
		llm.TextBlock(prompt),
		llm.ImageBlockFromBase64(imageData, mimeType),
	)

	resp, err := client.Chat(ctx, []llm.Message{msg}, nil)
	if err != nil {
		return "", fmt.Errorf("image analysis: %w", err)
	}

	result := map[string]any{
		"description": resp.Content,
		"source":      path,
		"prompt":      prompt,
		"usage": map[string]int{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		},
	}
	payload, _ := json.Marshal(result)
	return string(payload), nil
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
		return "image/jpeg"
	}
}
