package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
)

type ImageQuality int

const (
	QualityLow      ImageQuality = 30
	QualityMedium   ImageQuality = 60
	QualityHigh     ImageQuality = 85
	QualityLossless ImageQuality = 100
)

type ImageResizeMode string

const (
	ResizeFit       ImageResizeMode = "fit"
	ResizeFill      ImageResizeMode = "fill"
	ResizeStretch   ImageResizeMode = "stretch"
	ResizeThumbnail ImageResizeMode = "thumbnail"
)

type ImageOptions struct {
	Format        Format
	Quality       int
	MaxWidth      int
	MaxHeight     int
	ResizeMode    ImageResizeMode
	StripMetadata bool
}

func DefaultImageOptions() ImageOptions {
	return ImageOptions{
		Format:     FormatUnknown,
		Quality:    int(QualityHigh),
		ResizeMode: ResizeFit,
	}
}

type AudioOptions struct {
	Format     Format
	Bitrate    int
	SampleRate int
	Channels   int
	Codec      string
}

func DefaultAudioOptions() AudioOptions {
	return AudioOptions{
		Format:     FormatMP3,
		Bitrate:    128,
		SampleRate: 44100,
		Channels:   2,
	}
}

type VideoOptions struct {
	Format     Format
	VideoCodec string
	AudioCodec string
	Bitrate    string
	CRF        int
	MaxWidth   int
	MaxHeight  int
	FPS        float64
	Preset     string
}

func DefaultVideoOptions() VideoOptions {
	return VideoOptions{
		Format:     FormatMP4,
		VideoCodec: "libx264",
		AudioCodec: "aac",
		CRF:        23,
		Preset:     "medium",
	}
}

type Transcoder struct {
	ffmpegPath  string
	ffprobePath string
}

func NewTranscoder() *Transcoder {
	return &Transcoder{
		ffmpegPath:  "ffmpeg",
		ffprobePath: "ffprobe",
	}
}

func (t *Transcoder) SetFFmpegPath(path string) {
	t.ffmpegPath = path
}

func (t *Transcoder) SetFFprobePath(path string) {
	t.ffprobePath = path
}

func (t *Transcoder) CompressImage(data []byte, opts ImageOptions) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	if opts.MaxWidth > 0 || opts.MaxHeight > 0 {
		img = resizeImage(img, opts.MaxWidth, opts.MaxHeight, opts.ResizeMode)
	}

	targetFormat := opts.Format
	if targetFormat == FormatUnknown {
		targetFormat = formatFromString(format)
	}

	return encodeImage(img, targetFormat, opts.Quality)
}

func (t *Transcoder) ConvertImage(data []byte, targetFormat Format) ([]byte, error) {
	return t.CompressImage(data, ImageOptions{
		Format:  targetFormat,
		Quality: int(QualityHigh),
	})
}

func (t *Transcoder) ResizeImage(data []byte, width, height int, mode ImageResizeMode) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	resized := resizeImage(img, width, height, mode)
	return encodeImage(resized, formatFromString(format), int(QualityHigh))
}

func (t *Transcoder) StripImageMetadata(data []byte, format Format) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty image data")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	targetFormat := format
	if targetFormat == FormatUnknown {
		detected := DetectMediaType(data, "", "")
		if detected.Format != FormatUnknown {
			targetFormat = detected.Format
		} else {
			targetFormat = FormatPNG
		}
	}

	return encodeImage(img, targetFormat, int(QualityLossless))
}

func (t *Transcoder) TranscodeAudio(ctx context.Context, data []byte, opts AudioOptions) ([]byte, error) {
	if !t.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available for audio transcoding")
	}

	tmpIn, err := t.writeTempFile(data, "input")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	outExt := formatToExt(opts.Format)
	if outExt == "" {
		outExt = ".mp3"
	}

	tmpOut, err := t.createTempFile("output" + outExt)
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	outPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(outPath)

	args := t.buildAudioArgs(tmpIn, outPath, opts)

	cmd := exec.CommandContext(ctx, t.ffmpegPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg audio transcode: %w", err)
	}

	result, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read transcoded audio: %w", err)
	}

	return result, nil
}

func (t *Transcoder) TranscodeVideo(ctx context.Context, data []byte, opts VideoOptions) ([]byte, error) {
	if !t.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available for video transcoding")
	}

	tmpIn, err := t.writeTempFile(data, "input")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	outExt := formatToExt(opts.Format)
	if outExt == "" {
		outExt = ".mp4"
	}

	tmpOut, err := t.createTempFile("output" + outExt)
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	outPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(outPath)

	args := t.buildVideoArgs(tmpIn, outPath, opts)

	cmd := exec.CommandContext(ctx, t.ffmpegPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg video transcode: %w", err)
	}

	result, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read transcoded video: %w", err)
	}

	return result, nil
}

func (t *Transcoder) CompressAudio(ctx context.Context, data []byte, bitrate int) ([]byte, error) {
	opts := DefaultAudioOptions()
	opts.Bitrate = bitrate
	return t.TranscodeAudio(ctx, data, opts)
}

func (t *Transcoder) CompressVideo(ctx context.Context, data []byte, crf int) ([]byte, error) {
	opts := DefaultVideoOptions()
	opts.CRF = crf
	return t.TranscodeVideo(ctx, data, opts)
}

func (t *Transcoder) ExtractThumbnail(ctx context.Context, videoData []byte, timestamp string) ([]byte, error) {
	if !t.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available for thumbnail extraction")
	}

	if timestamp == "" {
		timestamp = "00:00:01"
	}

	tmpIn, err := t.writeTempFile(videoData, "video")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	tmpOut, err := t.createTempFile("thumb.jpg")
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	outPath := tmpOut.Name()
	tmpOut.Close()
	defer os.Remove(outPath)

	args := []string{
		"-i", tmpIn,
		"-ss", timestamp,
		"-vframes", "1",
		"-q:v", "2",
		"-y",
		outPath,
	}

	cmd := exec.CommandContext(ctx, t.ffmpegPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg thumbnail: %w", err)
	}

	return os.ReadFile(outPath)
}

func (t *Transcoder) ProbeMedia(ctx context.Context, data []byte) (map[string]any, error) {
	if !t.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffprobe not available")
	}

	tmpIn, err := t.writeTempFile(data, "probe")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		tmpIn,
	}

	cmd := exec.CommandContext(ctx, t.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	return result, nil
}

func (t *Transcoder) IsFFmpegAvailable(ctx context.Context) bool {
	return t.isFFmpegAvailable(ctx)
}

func (t *Transcoder) isFFmpegAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, t.ffmpegPath, "-version")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func (t *Transcoder) buildAudioArgs(input, output string, opts AudioOptions) []string {
	args := []string{
		"-i", input,
		"-y",
	}

	if opts.Codec != "" {
		args = append(args, "-acodec", opts.Codec)
	} else {
		args = append(args, "-acodec", audioCodecForFormat(opts.Format))
	}

	if opts.Bitrate > 0 {
		args = append(args, "-b:a", fmt.Sprintf("%dk", opts.Bitrate))
	}

	if opts.SampleRate > 0 {
		args = append(args, "-ar", fmt.Sprintf("%d", opts.SampleRate))
	}

	if opts.Channels > 0 {
		args = append(args, "-ac", fmt.Sprintf("%d", opts.Channels))
	}

	args = append(args, output)
	return args
}

func (t *Transcoder) buildVideoArgs(input, output string, opts VideoOptions) []string {
	args := []string{
		"-i", input,
		"-y",
	}

	if opts.VideoCodec != "" {
		args = append(args, "-c:v", opts.VideoCodec)
	}

	if opts.AudioCodec != "" {
		args = append(args, "-c:a", opts.AudioCodec)
	}

	if opts.CRF > 0 && opts.VideoCodec == "libx264" {
		args = append(args, "-crf", fmt.Sprintf("%d", opts.CRF))
	}

	if opts.Preset != "" && opts.VideoCodec == "libx264" {
		args = append(args, "-preset", opts.Preset)
	}

	if opts.Bitrate != "" {
		args = append(args, "-b:v", opts.Bitrate)
	}

	if opts.MaxWidth > 0 || opts.MaxHeight > 0 {
		w := opts.MaxWidth
		h := opts.MaxHeight
		if w == 0 {
			w = -2
		}
		if h == 0 {
			h = -2
		}
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", w, h))
	}

	if opts.FPS > 0 {
		args = append(args, "-r", fmt.Sprintf("%.2f", opts.FPS))
	}

	args = append(args, output)
	return args
}

func (t *Transcoder) writeTempFile(data []byte, prefix string) (string, error) {
	f, err := t.createTempFile(prefix)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

func (t *Transcoder) createTempFile(pattern string) (*os.File, error) {
	dir := os.TempDir()
	return os.CreateTemp(dir, fmt.Sprintf("anyclaw-media-%s-*", pattern))
}

func resizeImage(img image.Image, maxWidth, maxHeight int, mode ImageResizeMode) image.Image {
	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	if maxWidth <= 0 {
		maxWidth = origW
	}
	if maxHeight <= 0 {
		maxHeight = origH
	}

	var newW, newH int

	switch mode {
	case ResizeFit:
		scale := float64(maxWidth) / float64(origW)
		if float64(origH)*scale > float64(maxHeight) {
			scale = float64(maxHeight) / float64(origH)
		}
		newW = int(float64(origW) * scale)
		newH = int(float64(origH) * scale)
	case ResizeFill:
		scaleX := float64(maxWidth) / float64(origW)
		scaleY := float64(maxHeight) / float64(origH)
		scale := scaleX
		if scaleY > scaleX {
			scale = scaleY
		}
		newW = int(float64(origW) * scale)
		newH = int(float64(origH) * scale)
	case ResizeStretch:
		newW = maxWidth
		newH = maxHeight
	case ResizeThumbnail:
		size := maxWidth
		if maxHeight < maxWidth {
			size = maxHeight
		}
		scale := float64(size) / float64(origW)
		if float64(origH)*scale > float64(size) {
			scale = float64(size) / float64(origH)
		}
		newW = int(float64(origW) * scale)
		newH = int(float64(origH) * scale)
	default:
		return img
	}

	if newW <= 0 || newH <= 0 {
		return img
	}

	if newW == origW && newH == origH {
		return img
	}

	return scaleImageBilinear(img, newW, newH)
}

func scaleImageBilinear(src image.Image, newW, newH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW == 0 || srcH == 0 {
		return dst
	}

	xRatio := float64(srcW) / float64(newW)
	yRatio := float64(srcH) / float64(newH)

	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)

			if srcX >= srcW {
				srcX = srcW - 1
			}
			if srcY >= srcH {
				srcY = srcH - 1
			}

			r, g, b, a := src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY).RGBA()
			dst.Set(x, y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}

	return dst
}

func encodeImage(img image.Image, format Format, quality int) ([]byte, error) {
	var buf bytes.Buffer

	switch format {
	case FormatJPEG:
		if quality <= 0 {
			quality = int(QualityHigh)
		}
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
		return buf.Bytes(), err
	case FormatPNG:
		encoder := &png.Encoder{
			CompressionLevel: png.DefaultCompression,
		}
		err := encoder.Encode(&buf, img)
		return buf.Bytes(), err
	case FormatGIF:
		err := gif.Encode(&buf, img, &gif.Options{NumColors: 256})
		return buf.Bytes(), err
	default:
		err := png.Encode(&buf, img)
		return buf.Bytes(), err
	}
}

func formatFromString(goFormat string) Format {
	switch goFormat {
	case "jpeg":
		return FormatJPEG
	case "png":
		return FormatPNG
	case "gif":
		return FormatGIF
	default:
		return FormatPNG
	}
}

func formatToExt(format Format) string {
	switch format {
	case FormatMP3:
		return ".mp3"
	case FormatWAV:
		return ".wav"
	case FormatOGG:
		return ".ogg"
	case FormatFLAC:
		return ".flac"
	case FormatAAC:
		return ".aac"
	case FormatM4A:
		return ".m4a"
	case FormatMP4:
		return ".mp4"
	case FormatWebM:
		return ".webm"
	case FormatAVI:
		return ".avi"
	case FormatMKV:
		return ".mkv"
	case FormatMOV:
		return ".mov"
	case Format3GP:
		return ".3gp"
	default:
		return ""
	}
}

func audioCodecForFormat(format Format) string {
	switch format {
	case FormatMP3:
		return "libmp3lame"
	case FormatWAV:
		return "pcm_s16le"
	case FormatOGG:
		return "libvorbis"
	case FormatFLAC:
		return "flac"
	case FormatAAC, FormatM4A:
		return "aac"
	default:
		return "libmp3lame"
	}
}
