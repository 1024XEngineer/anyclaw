package tools

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestLocateTemplateFindsInsertedTemplate(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 96, 72))
	fillRect(source, source.Bounds(), color.NRGBA{R: 24, G: 28, B: 34, A: 255})

	template := image.NewNRGBA(image.Rect(0, 0, 14, 10))
	for y := 0; y < template.Bounds().Dy(); y++ {
		for x := 0; x < template.Bounds().Dx(); x++ {
			template.Set(x, y, color.NRGBA{
				R: uint8(20 + x*11),
				G: uint8(40 + y*13),
				B: uint8(120 + (x+y)*3),
				A: 255,
			})
		}
	}

	targetX, targetY := 31, 22
	drawInto(source, template, targetX, targetY)

	match, err := locateTemplate(source, template, image.Rect(0, 0, 96, 72), 2)
	if err != nil {
		t.Fatalf("locateTemplate: %v", err)
	}
	if match.X != targetX || match.Y != targetY {
		t.Fatalf("expected match at %d,%d got %d,%d", targetX, targetY, match.X, match.Y)
	}
	if match.Score < 0.99 {
		t.Fatalf("expected strong score, got %.4f", match.Score)
	}
}

func TestResolveSearchRectRejectsSmallArea(t *testing.T) {
	_, err := resolveSearchRect(map[string]any{
		"search_x":      0,
		"search_y":      0,
		"search_width":  8,
		"search_height": 8,
	}, image.Rect(0, 0, 100, 100), image.Rect(0, 0, 12, 12))
	if err == nil {
		t.Fatal("expected search area size validation error")
	}
}

func TestVerifyOCRTextModes(t *testing.T) {
	actual := "Task Completed\nExport finished"

	if !verifyOCRText(actual, "completed", "contains", true) {
		t.Fatal("expected contains match to succeed")
	}
	if !verifyOCRText(actual, "Task Completed\nExport finished", "exact", false) {
		t.Fatal("expected exact match to succeed")
	}
	if !verifyOCRText(actual, "export\\s+finished", "regex", true) {
		t.Fatal("expected regex match to succeed")
	}
	if verifyOCRText(actual, "failed", "contains", true) {
		t.Fatal("expected missing text check to fail")
	}
}

func TestNormalizeOCRTextRemovesNoise(t *testing.T) {
	actual := " Hello \r\n\r\nWorld \u000c"
	if normalized := normalizeOCRText(actual); normalized != "Hello\nWorld" {
		t.Fatalf("unexpected normalized OCR text: %q", normalized)
	}
}

func TestParseTesseractTSVAndFindTextMatch(t *testing.T) {
	tsv := strings.Join([]string{
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
		"5\t1\t1\t1\t1\t1\t100\t200\t50\t20\t96\tExport",
		"5\t1\t1\t1\t1\t2\t156\t200\t62\t20\t93\tPNG",
		"5\t1\t1\t1\t2\t1\t100\t240\t80\t20\t88\tCancel",
	}, "\n")

	words, err := parseTesseractTSV(tsv)
	if err != nil {
		t.Fatalf("parseTesseractTSV: %v", err)
	}
	if len(words) != 3 {
		t.Fatalf("expected 3 OCR words, got %d", len(words))
	}

	lines := buildOCRLineBoxes(words, 30)
	if len(lines) != 2 {
		t.Fatalf("expected 2 OCR lines, got %d", len(lines))
	}

	match := findOCRTextMatch(lines, words, "Export PNG", "contains", true, 1, 30)
	if !match.Found {
		t.Fatal("expected Export PNG line match")
	}
	if match.Scope != "line" {
		t.Fatalf("expected line scope, got %q", match.Scope)
	}
	if match.X != 100 || match.Y != 200 {
		t.Fatalf("unexpected line bounds: %#v", match)
	}
	if match.CenterX <= 0 || match.CenterY <= 0 {
		t.Fatalf("expected valid click center: %#v", match)
	}
}

func TestFindOCRTextMatchFallsBackToWord(t *testing.T) {
	words := []ocrWordBox{
		{Text: "Open", Confidence: 92, X: 10, Y: 10, Width: 40, Height: 18, CenterX: 30, CenterY: 19, Key: "1:1:1", WordNum: 1},
		{Text: "File", Confidence: 95, X: 60, Y: 10, Width: 30, Height: 18, CenterX: 75, CenterY: 19, Key: "1:1:1", WordNum: 2},
	}
	lines := buildOCRLineBoxes(words, 30)

	match := findOCRTextMatch(lines, words, "^File$", "regex", true, 1, 30)
	if !match.Found {
		t.Fatal("expected regex word match")
	}
	if match.Scope != "word" {
		t.Fatalf("expected word scope, got %q", match.Scope)
	}
	if match.X != 60 || match.Y != 10 {
		t.Fatalf("unexpected word bounds: %#v", match)
	}
}

func fillRect(img *image.NRGBA, rect image.Rectangle, c color.NRGBA) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func drawInto(dst *image.NRGBA, src *image.NRGBA, offsetX int, offsetY int) {
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			dst.Set(offsetX+x, offsetY+y, src.At(x, y))
		}
	}
}
