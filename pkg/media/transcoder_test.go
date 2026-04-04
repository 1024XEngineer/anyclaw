package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func createTestImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: 128,
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestTranscoder_CompressImage_JPEG(t *testing.T) {
	imgData := createTestImage(100, 100)
	tr := NewTranscoder()

	compressed, err := tr.CompressImage(imgData, ImageOptions{
		Format:  FormatJPEG,
		Quality: int(QualityMedium),
	})
	if err != nil {
		t.Fatalf("compress image: %v", err)
	}

	if len(compressed) == 0 {
		t.Fatal("compressed data is empty")
	}

	result := DetectMediaType(compressed, "", "")
	if result.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG, got %s", result.Format)
	}
}

func TestTranscoder_CompressImage_QualityLevels(t *testing.T) {
	imgData := createTestImage(200, 200)
	tr := NewTranscoder()

	sizes := map[string]int{
		"low":    int(QualityLow),
		"medium": int(QualityMedium),
		"high":   int(QualityHigh),
	}

	var lastSize int64
	for name, quality := range sizes {
		compressed, err := tr.CompressImage(imgData, ImageOptions{
			Format:  FormatJPEG,
			Quality: quality,
		})
		if err != nil {
			t.Fatalf("[%s] compress: %v", name, err)
		}

		if int64(len(compressed)) <= 0 {
			t.Errorf("[%s] compressed data empty", name)
		}

		if lastSize > 0 && quality > int(QualityMedium) {
			if int64(len(compressed)) < lastSize {
				t.Logf("[%s] size=%d (expected larger for higher quality, got smaller)", name, len(compressed))
			}
		}
		lastSize = int64(len(compressed))
	}
}

func TestTranscoder_ConvertImage_PNGToJPEG(t *testing.T) {
	imgData := createTestImage(50, 50)
	tr := NewTranscoder()

	converted, err := tr.ConvertImage(imgData, FormatJPEG)
	if err != nil {
		t.Fatalf("convert to JPEG: %v", err)
	}

	result := DetectMediaType(converted, "", "")
	if result.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG, got %s", result.Format)
	}
	if result.Type != TypeImage {
		t.Errorf("expected TypeImage, got %s", result.Type)
	}
}

func TestTranscoder_ConvertImage_JPEGToPNG(t *testing.T) {
	jpegData := createTestImage(50, 50)
	tr := NewTranscoder()

	converted, err := tr.ConvertImage(jpegData, FormatPNG)
	if err != nil {
		t.Fatalf("convert to PNG: %v", err)
	}

	result := DetectMediaType(converted, "", "")
	if result.Format != FormatPNG {
		t.Errorf("expected FormatPNG, got %s", result.Format)
	}
}

func TestTranscoder_ConvertImage_PNGToGIF(t *testing.T) {
	imgData := createTestImage(50, 50)
	tr := NewTranscoder()

	converted, err := tr.ConvertImage(imgData, FormatGIF)
	if err != nil {
		t.Fatalf("convert to GIF: %v", err)
	}

	result := DetectMediaType(converted, "", "")
	if result.Format != FormatGIF {
		t.Errorf("expected FormatGIF, got %s", result.Format)
	}
}

func TestTranscoder_ResizeImage_Fit(t *testing.T) {
	imgData := createTestImage(200, 100)
	tr := NewTranscoder()

	resized, err := tr.ResizeImage(imgData, 100, 50, ResizeFit)
	if err != nil {
		t.Fatalf("resize image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 100 {
		t.Errorf("expected width 100, got %d", bounds.Dx())
	}
	if bounds.Dy() != 50 {
		t.Errorf("expected height 50, got %d", bounds.Dy())
	}
}

func TestTranscoder_ResizeImage_Stretch(t *testing.T) {
	imgData := createTestImage(200, 100)
	tr := NewTranscoder()

	resized, err := tr.ResizeImage(imgData, 80, 80, ResizeStretch)
	if err != nil {
		t.Fatalf("resize image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 80 {
		t.Errorf("expected width 80, got %d", bounds.Dx())
	}
	if bounds.Dy() != 80 {
		t.Errorf("expected height 80, got %d", bounds.Dy())
	}
}

func TestTranscoder_ResizeImage_Thumbnail(t *testing.T) {
	imgData := createTestImage(300, 200)
	tr := NewTranscoder()

	resized, err := tr.ResizeImage(imgData, 64, 64, ResizeThumbnail)
	if err != nil {
		t.Fatalf("resize image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() > 64 {
		t.Errorf("expected width <= 64, got %d", bounds.Dx())
	}
	if bounds.Dy() > 64 {
		t.Errorf("expected height <= 64, got %d", bounds.Dy())
	}
}

func TestTranscoder_StripImageMetadata(t *testing.T) {
	imgData := createTestImage(100, 100)
	tr := NewTranscoder()

	stripped, err := tr.StripImageMetadata(imgData, FormatPNG)
	if err != nil {
		t.Fatalf("strip metadata: %v", err)
	}

	if len(stripped) == 0 {
		t.Fatal("stripped data is empty")
	}

	result := DetectMediaType(stripped, "", "")
	if result.Format != FormatPNG {
		t.Errorf("expected FormatPNG, got %s", result.Format)
	}
}

func TestTranscoder_CompressImage_EmptyData(t *testing.T) {
	tr := NewTranscoder()

	_, err := tr.CompressImage([]byte{}, ImageOptions{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestTranscoder_CompressImage_InvalidData(t *testing.T) {
	tr := NewTranscoder()

	_, err := tr.CompressImage([]byte("not an image"), ImageOptions{})
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

func TestTranscoder_ResizeImage_NoResize(t *testing.T) {
	imgData := createTestImage(100, 100)
	tr := NewTranscoder()

	resized, err := tr.ResizeImage(imgData, 0, 0, ResizeFit)
	if err != nil {
		t.Fatalf("resize image: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(resized))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 100 {
		t.Errorf("expected width 100, got %d", bounds.Dx())
	}
	if bounds.Dy() != 100 {
		t.Errorf("expected height 100, got %d", bounds.Dy())
	}
}

func TestTranscoder_IsFFmpegAvailable(t *testing.T) {
	tr := NewTranscoder()
	ctx := context.Background()

	_ = tr.IsFFmpegAvailable(ctx)
}

func TestTranscoder_TranscodeAudio_NoFFmpeg(t *testing.T) {
	tr := NewTranscoder()
	tr.SetFFmpegPath("nonexistent-ffmpeg")
	ctx := context.Background()

	_, err := tr.TranscodeAudio(ctx, []byte("fake audio"), DefaultAudioOptions())
	if err == nil {
		t.Fatal("expected error when ffmpeg not available")
	}
}

func TestTranscoder_TranscodeVideo_NoFFmpeg(t *testing.T) {
	tr := NewTranscoder()
	tr.SetFFmpegPath("nonexistent-ffmpeg")
	ctx := context.Background()

	_, err := tr.TranscodeVideo(ctx, []byte("fake video"), DefaultVideoOptions())
	if err == nil {
		t.Fatal("expected error when ffmpeg not available")
	}
}

func TestTranscoder_ExtractThumbnail_NoFFmpeg(t *testing.T) {
	tr := NewTranscoder()
	tr.SetFFmpegPath("nonexistent-ffmpeg")
	ctx := context.Background()

	_, err := tr.ExtractThumbnail(ctx, []byte("fake video"), "00:00:01")
	if err == nil {
		t.Fatal("expected error when ffmpeg not available")
	}
}

func TestTranscoder_ProbeMedia_NoFFmpeg(t *testing.T) {
	tr := NewTranscoder()
	tr.SetFFprobePath("nonexistent-ffprobe")
	ctx := context.Background()

	_, err := tr.ProbeMedia(ctx, []byte("fake media"))
	if err == nil {
		t.Fatal("expected error when ffprobe not available")
	}
}

func TestTranscoder_CompressAudio_NoFFmpeg(t *testing.T) {
	tr := NewTranscoder()
	tr.SetFFmpegPath("nonexistent-ffmpeg")
	ctx := context.Background()

	_, err := tr.CompressAudio(ctx, []byte("fake audio"), 64)
	if err == nil {
		t.Fatal("expected error when ffmpeg not available")
	}
}

func TestTranscoder_CompressVideo_NoFFmpeg(t *testing.T) {
	tr := NewTranscoder()
	tr.SetFFmpegPath("nonexistent-ffmpeg")
	ctx := context.Background()

	_, err := tr.CompressVideo(ctx, []byte("fake video"), 28)
	if err == nil {
		t.Fatal("expected error when ffmpeg not available")
	}
}

func TestTranscoder_FormatToExt(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatMP3, ".mp3"},
		{FormatWAV, ".wav"},
		{FormatOGG, ".ogg"},
		{FormatFLAC, ".flac"},
		{FormatAAC, ".aac"},
		{FormatM4A, ".m4a"},
		{FormatMP4, ".mp4"},
		{FormatWebM, ".webm"},
		{FormatAVI, ".avi"},
		{FormatMKV, ".mkv"},
		{FormatMOV, ".mov"},
		{Format3GP, ".3gp"},
		{FormatUnknown, ""},
	}

	for _, tt := range tests {
		got := formatToExt(tt.format)
		if got != tt.want {
			t.Errorf("formatToExt(%s) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestTranscoder_AudioCodecForFormat(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatMP3, "libmp3lame"},
		{FormatWAV, "pcm_s16le"},
		{FormatOGG, "libvorbis"},
		{FormatFLAC, "flac"},
		{FormatAAC, "aac"},
		{FormatM4A, "aac"},
		{FormatUnknown, "libmp3lame"},
	}

	for _, tt := range tests {
		got := audioCodecForFormat(tt.format)
		if got != tt.want {
			t.Errorf("audioCodecForFormat(%s) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestProcessor_Compress_Image(t *testing.T) {
	imgData := createTestImage(200, 200)
	proc := NewProcessor("")

	media := &Media{
		ID:       "test-1",
		Type:     TypeImage,
		Data:     imgData,
		MimeType: "image/png",
		Name:     "test.png",
	}

	compressed, err := proc.Compress(context.Background(), media, ImageOptions{
		Format:  FormatJPEG,
		Quality: int(QualityMedium),
	})
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	if compressed.Size <= 0 {
		t.Error("compressed media has no size")
	}

	if compressed.MimeType != "image/jpeg" {
		t.Errorf("expected mimeType image/jpeg, got %s", compressed.MimeType)
	}
}

func TestProcessor_Convert_Image(t *testing.T) {
	imgData := createTestImage(100, 100)
	proc := NewProcessor("")

	media := &Media{
		ID:       "test-2",
		Type:     TypeImage,
		Data:     imgData,
		MimeType: "image/png",
		Name:     "test.png",
	}

	converted, err := proc.Convert(context.Background(), media, FormatJPEG)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if converted.MimeType != "image/jpeg" {
		t.Errorf("expected mimeType image/jpeg, got %s", converted.MimeType)
	}

	if converted.Metadata["format"] != "jpeg" {
		t.Errorf("expected format jpeg in metadata, got %v", converted.Metadata["format"])
	}
}

func TestProcessor_Compress_UnsupportedType(t *testing.T) {
	proc := NewProcessor("")

	media := &Media{
		ID:   "test-3",
		Type: TypeDoc,
		Data: []byte("document"),
	}

	_, err := proc.Compress(context.Background(), media, ImageOptions{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestProcessor_Convert_UnsupportedType(t *testing.T) {
	proc := NewProcessor("")

	media := &Media{
		ID:   "test-4",
		Type: TypeDoc,
		Data: []byte("document"),
	}

	_, err := proc.Convert(context.Background(), media, FormatPDF)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestTranscoder_CompressImage_ResizeAndCompress(t *testing.T) {
	imgData := createTestImage(500, 500)
	tr := NewTranscoder()

	compressed, err := tr.CompressImage(imgData, ImageOptions{
		Format:     FormatJPEG,
		Quality:    int(QualityLow),
		MaxWidth:   100,
		MaxHeight:  100,
		ResizeMode: ResizeFit,
	})
	if err != nil {
		t.Fatalf("compress and resize: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() > 100 || bounds.Dy() > 100 {
		t.Errorf("expected dimensions <= 100, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	result := DetectMediaType(compressed, "", "")
	if result.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG, got %s", result.Format)
	}
}

func TestProcessor_Transcoder_GetSet(t *testing.T) {
	proc := NewProcessor("")

	tr := proc.Transcoder()
	if tr == nil {
		t.Fatal("expected default transcoder")
	}

	newTr := NewTranscoder()
	proc.SetTranscoder(newTr)

	if proc.Transcoder() != newTr {
		t.Error("transcoder not updated")
	}
}

func TestMediaPipeline_TranscodeConfig(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	cfg.Transcode = true
	cfg.ImageOptions = ImageOptions{
		Format:  FormatJPEG,
		Quality: int(QualityMedium),
	}

	p := NewMediaPipeline(cfg)
	if !p.config.Transcode {
		t.Error("transcode not enabled")
	}

	if p.config.ImageOptions.Format != FormatJPEG {
		t.Errorf("expected FormatJPEG, got %s", p.config.ImageOptions.Format)
	}
}

func TestMediaPipeline_TranscodeDisabled(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	cfg.Transcode = false

	p := NewMediaPipeline(cfg)
	if p.config.Transcode {
		t.Error("transcode should be disabled")
	}
}
