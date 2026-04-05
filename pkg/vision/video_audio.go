package vision

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type KeyFrame struct {
	Timestamp float64 `json:"timestamp"`
	FrameData []byte  `json:"-"`
	Index     int     `json:"index"`
	SceneID   int     `json:"scene_id"`
}

type SceneInfo struct {
	ID        int        `json:"id"`
	Start     float64    `json:"start"`
	End       float64    `json:"end"`
	Duration  float64    `json:"duration"`
	KeyFrames []KeyFrame `json:"key_frames"`
}

type VideoAnalysisResult struct {
	Duration    float64        `json:"duration"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	FPS         float64        `json:"fps"`
	Codec       string         `json:"codec"`
	Scenes      []SceneInfo    `json:"scenes"`
	KeyFrames   []KeyFrame     `json:"key_frames"`
	TotalFrames int            `json:"total_frames"`
	Metadata    map[string]any `json:"metadata"`
}

type KeyFrameExtractor struct {
	ffmpegPath     string
	ffprobePath    string
	sceneThreshold float64
	maxKeyFrames   int
}

func NewKeyFrameExtractor() *KeyFrameExtractor {
	return &KeyFrameExtractor{
		ffmpegPath:     "ffmpeg",
		ffprobePath:    "ffprobe",
		sceneThreshold: 30.0,
		maxKeyFrames:   20,
	}
}

func (e *KeyFrameExtractor) SetFFmpegPath(path string) {
	e.ffmpegPath = path
}

func (e *KeyFrameExtractor) SetFFprobePath(path string) {
	e.ffprobePath = path
}

func (e *KeyFrameExtractor) SetSceneThreshold(threshold float64) {
	e.sceneThreshold = threshold
}

func (e *KeyFrameExtractor) SetMaxKeyFrames(n int) {
	e.maxKeyFrames = n
}

func (e *KeyFrameExtractor) ExtractKeyFrames(ctx context.Context, videoData []byte) (*VideoAnalysisResult, error) {
	if !e.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available")
	}

	tmpIn, err := e.writeTempFile(videoData, "video")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	meta, err := e.probeVideo(ctx, tmpIn)
	if err != nil {
		return nil, fmt.Errorf("probe video: %w", err)
	}

	scenes, err := e.detectScenes(ctx, tmpIn, meta.Duration)
	if err != nil {
		scenes = []SceneInfo{
			{ID: 0, Start: 0, End: meta.Duration, Duration: meta.Duration},
		}
	}

	keyFrames := make([]KeyFrame, 0)
	for _, scene := range scenes {
		midTime := scene.Start + scene.Duration/2
		frameData, err := e.extractFrameAt(ctx, tmpIn, midTime)
		if err != nil {
			continue
		}

		kf := KeyFrame{
			Timestamp: midTime,
			FrameData: frameData,
			Index:     len(keyFrames),
			SceneID:   scene.ID,
		}
		keyFrames = append(keyFrames, kf)
		scene.KeyFrames = append(scene.KeyFrames, kf)

		if len(keyFrames) >= e.maxKeyFrames {
			break
		}
	}

	if len(keyFrames) == 0 {
		frameData, err := e.extractFrameAt(ctx, tmpIn, 1.0)
		if err == nil {
			keyFrames = append(keyFrames, KeyFrame{
				Timestamp: 1.0,
				FrameData: frameData,
				Index:     0,
				SceneID:   0,
			})
		}
	}

	meta.KeyFrames = keyFrames
	meta.Scenes = scenes
	return meta, nil
}

func (e *KeyFrameExtractor) ExtractFrameAt(ctx context.Context, videoData []byte, timestamp float64) ([]byte, error) {
	if !e.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available")
	}

	tmpIn, err := e.writeTempFile(videoData, "video")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	return e.extractFrameAt(ctx, tmpIn, timestamp)
}

func (e *KeyFrameExtractor) ExtractFramesAtIntervals(ctx context.Context, videoData []byte, intervalSeconds float64) ([][]byte, error) {
	if !e.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available")
	}

	tmpIn, err := e.writeTempFile(videoData, "video")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	meta, err := e.probeVideo(ctx, tmpIn)
	if err != nil {
		return nil, fmt.Errorf("probe video: %w", err)
	}

	var frames [][]byte
	for t := 0.0; t < meta.Duration; t += intervalSeconds {
		frameData, err := e.extractFrameAt(ctx, tmpIn, t)
		if err != nil {
			continue
		}
		frames = append(frames, frameData)
	}

	return frames, nil
}

func (e *KeyFrameExtractor) detectScenes(ctx context.Context, input string, duration float64) ([]SceneInfo, error) {
	args := []string{
		"-i", input,
		"-vf", fmt.Sprintf("select='gt(scene,%f)',showinfo", e.sceneThreshold/100.0),
		"-vsync", "vfr",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, e.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("scene detection: %w", err)
	}

	var sceneChanges []float64
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "pts_time:") {
			parts := strings.Split(line, "pts_time:")
			if len(parts) >= 2 {
				var t float64
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &t)
				if t > 0 {
					sceneChanges = append(sceneChanges, t)
				}
			}
		}
	}

	if len(sceneChanges) == 0 {
		return nil, fmt.Errorf("no scene changes detected")
	}

	sort.Float64s(sceneChanges)

	scenes := make([]SceneInfo, 0, len(sceneChanges)+1)
	sceneID := 0
	prevTime := 0.0

	for _, changeTime := range sceneChanges {
		scenes = append(scenes, SceneInfo{
			ID:       sceneID,
			Start:    prevTime,
			End:      changeTime,
			Duration: changeTime - prevTime,
		})
		prevTime = changeTime
		sceneID++
	}

	if prevTime < duration {
		scenes = append(scenes, SceneInfo{
			ID:       sceneID,
			Start:    prevTime,
			End:      duration,
			Duration: duration - prevTime,
		})
	}

	return scenes, nil
}

func (e *KeyFrameExtractor) extractFrameAt(ctx context.Context, input string, timestamp float64) ([]byte, error) {
	args := []string{
		"-i", input,
		"-ss", fmt.Sprintf("%.3f", timestamp),
		"-vframes", "1",
		"-q:v", "2",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-",
	}

	cmd := exec.CommandContext(ctx, e.ffmpegPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("extract frame at %.3f: %w", timestamp, err)
	}

	if out.Len() == 0 {
		return nil, fmt.Errorf("empty frame at %.3f", timestamp)
	}

	return out.Bytes(), nil
}

func (e *KeyFrameExtractor) probeVideo(ctx context.Context, input string) (*VideoAnalysisResult, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		input,
	}

	cmd := exec.CommandContext(ctx, e.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var probeResult struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
			Duration   string `json:"duration"`
			NbFrames   string `json:"nb_frames"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
		} `json:"format"`
	}

	if err := jsonUnmarshal(output, &probeResult); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	result := &VideoAnalysisResult{Metadata: make(map[string]any)}

	for _, stream := range probeResult.Streams {
		if stream.CodecType == "video" {
			result.Width = stream.Width
			result.Height = stream.Height
			result.Codec = stream.CodecName
			result.FPS = parseFPS(stream.RFrameRate)
			if stream.NbFrames != "" {
				fmt.Sscanf(stream.NbFrames, "%d", &result.TotalFrames)
			}
		}
	}

	if probeResult.Format.Duration != "" {
		fmt.Sscanf(probeResult.Format.Duration, "%f", &result.Duration)
	}

	return result, nil
}

func (e *KeyFrameExtractor) isFFmpegAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, e.ffmpegPath, "-version")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func (e *KeyFrameExtractor) writeTempFile(data []byte, prefix string) (string, error) {
	f, err := os.CreateTemp(os.TempDir(), fmt.Sprintf("anyclaw-vision-%s-*", prefix))
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

func parseFPS(fpsStr string) float64 {
	if idx := strings.Index(fpsStr, "/"); idx > 0 {
		var num, den float64
		fmt.Sscanf(fpsStr[:idx], "%f", &num)
		fmt.Sscanf(fpsStr[idx+1:], "%f", &den)
		if den > 0 {
			return num / den
		}
	}
	var fps float64
	fmt.Sscanf(fpsStr, "%f", &fps)
	return fps
}

type AudioAnalysisResult struct {
	Duration         float64          `json:"duration"`
	SampleRate       int              `json:"sample_rate"`
	Channels         int              `json:"channels"`
	Codec            string           `json:"codec"`
	Bitrate          int              `json:"bitrate"`
	IsSpeech         bool             `json:"is_speech"`
	SpeechConfidence float64          `json:"speech_confidence"`
	IsMusic          bool             `json:"is_music"`
	MusicConfidence  float64          `json:"music_confidence"`
	EnergyProfile    []float64        `json:"energy_profile"`
	SilenceSegments  []SilenceSegment `json:"silence_segments"`
	Metadata         map[string]any   `json:"metadata"`
}

type SilenceSegment struct {
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Duration float64 `json:"duration"`
}

type AudioAnalyzer struct {
	ffmpegPath  string
	ffprobePath string
}

func NewAudioAnalyzer() *AudioAnalyzer {
	return &AudioAnalyzer{
		ffmpegPath:  "ffmpeg",
		ffprobePath: "ffprobe",
	}
}

func (a *AudioAnalyzer) SetFFmpegPath(path string) {
	a.ffmpegPath = path
}

func (a *AudioAnalyzer) SetFFprobePath(path string) {
	a.ffprobePath = path
}

func (a *AudioAnalyzer) Analyze(ctx context.Context, audioData []byte) (*AudioAnalysisResult, error) {
	if !a.isFFmpegAvailable(ctx) {
		return nil, fmt.Errorf("ffmpeg not available")
	}

	tmpIn, err := a.writeTempFile(audioData, "audio")
	if err != nil {
		return nil, fmt.Errorf("create temp input: %w", err)
	}
	defer os.Remove(tmpIn)

	result := &AudioAnalysisResult{Metadata: make(map[string]any)}

	meta, err := a.probeAudio(ctx, tmpIn)
	if err != nil {
		return nil, fmt.Errorf("probe audio: %w", err)
	}

	result.Duration = meta.Duration
	result.SampleRate = meta.SampleRate
	result.Channels = meta.Channels
	result.Codec = meta.Codec
	result.Bitrate = meta.Bitrate

	energyProfile, err := a.extractEnergyProfile(ctx, tmpIn)
	if err == nil {
		result.EnergyProfile = energyProfile
	}

	silenceSegments, err := a.detectSilence(ctx, tmpIn)
	if err == nil {
		result.SilenceSegments = silenceSegments
	}

	speechConfidence, err := a.detectSpeech(ctx, tmpIn)
	if err == nil {
		result.SpeechConfidence = speechConfidence
		result.IsSpeech = speechConfidence > 0.5
	}

	musicConfidence, err := a.detectMusic(ctx, tmpIn, energyProfile)
	if err == nil {
		result.MusicConfidence = musicConfidence
		result.IsMusic = musicConfidence > 0.5
	}

	if result.IsSpeech && result.IsMusic {
		if result.SpeechConfidence > result.MusicConfidence {
			result.IsMusic = false
		} else {
			result.IsSpeech = false
		}
	}

	result.Metadata["silence_ratio"] = a.calcSilenceRatio(silenceSegments, result.Duration)
	result.Metadata["energy_variance"] = a.calcEnergyVariance(energyProfile)

	return result, nil
}

func (a *AudioAnalyzer) extractEnergyProfile(ctx context.Context, input string) ([]float64, error) {
	args := []string{
		"-i", input,
		"-af", "astats=metadata=1:reset=1",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("energy extraction: %w", err)
	}

	var energies []float64
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "RMS level dB") {
			var db float64
			fmt.Sscanf(line, "%*[^=]= %f", &db)
			normalized := math.Pow(10, db/20.0)
			energies = append(energies, normalized)
		}
	}

	return energies, nil
}

func (a *AudioAnalyzer) detectSilence(ctx context.Context, input string) ([]SilenceSegment, error) {
	args := []string{
		"-i", input,
		"-af", "silencedetect=noise=-30dB:d=0.5",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("silence detection: %w", err)
	}

	var segments []SilenceSegment
	lines := strings.Split(string(output), "\n")

	var currentStart float64
	inSilence := false

	for _, line := range lines {
		if strings.Contains(line, "silence_start:") {
			var t float64
			fmt.Sscanf(line, "%*[^:]: %f", &t)
			currentStart = t
			inSilence = true
		} else if strings.Contains(line, "silence_end:") && inSilence {
			var end, duration float64
			fmt.Sscanf(line, "%*[^:]: %f %*[^:]: %f", &end, &duration)
			segments = append(segments, SilenceSegment{
				Start:    currentStart,
				End:      end,
				Duration: duration,
			})
			inSilence = false
		}
	}

	return segments, nil
}

func (a *AudioAnalyzer) detectSpeech(ctx context.Context, input string) (float64, error) {
	args := []string{
		"-i", input,
		"-af", "highpass=f=300,lowpass=f=3000,astats=metadata=1",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("speech detection: %w", err)
	}

	var rmsValues []float64
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "RMS level dB") {
			var db float64
			fmt.Sscanf(line, "%*[^=]= %f", &db)
			rmsValues = append(rmsValues, math.Pow(10, db/20.0))
		}
	}

	if len(rmsValues) == 0 {
		return 0, nil
	}

	avgEnergy := 0.0
	for _, e := range rmsValues {
		avgEnergy += e
	}
	avgEnergy /= float64(len(rmsValues))

	if avgEnergy > 0.01 && avgEnergy < 0.5 {
		return math.Min(1.0, avgEnergy*5), nil
	}

	return avgEnergy * 2, nil
}

func (a *AudioAnalyzer) detectMusic(ctx context.Context, input string, energyProfile []float64) (float64, error) {
	args := []string{
		"-i", input,
		"-af", "highpass=f=20,astats=metadata=1",
		"-f", "null",
		"-",
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("music detection: %w", err)
	}

	var rmsValues []float64
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "RMS level dB") {
			var db float64
			fmt.Sscanf(line, "%*[^=]= %f", &db)
			rmsValues = append(rmsValues, math.Pow(10, db/20.0))
		}
	}

	if len(rmsValues) < 2 {
		return 0, nil
	}

	avgEnergy := 0.0
	for _, e := range rmsValues {
		avgEnergy += e
	}
	avgEnergy /= float64(len(rmsValues))

	if avgEnergy > 0.05 {
		return math.Min(1.0, avgEnergy*3), nil
	}

	return avgEnergy, nil
}

func (a *AudioAnalyzer) probeAudio(ctx context.Context, input string) (*AudioAnalysisResult, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		input,
	}

	cmd := exec.CommandContext(ctx, a.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}

	var probeResult struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			BitRate    string `json:"bit_rate"`
			Duration   string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
	}

	if err := jsonUnmarshal(output, &probeResult); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	result := &AudioAnalysisResult{}

	for _, stream := range probeResult.Streams {
		if stream.CodecType == "audio" {
			result.Codec = stream.CodecName
			result.Channels = stream.Channels
			fmt.Sscanf(stream.SampleRate, "%d", &result.SampleRate)
			if stream.BitRate != "" {
				fmt.Sscanf(stream.BitRate, "%d", &result.Bitrate)
			}
			if stream.Duration != "" {
				fmt.Sscanf(stream.Duration, "%f", &result.Duration)
			}
		}
	}

	if result.Duration <= 0 && probeResult.Format.Duration != "" {
		fmt.Sscanf(probeResult.Format.Duration, "%f", &result.Duration)
	}
	if result.Bitrate <= 0 && probeResult.Format.BitRate != "" {
		fmt.Sscanf(probeResult.Format.BitRate, "%d", &result.Bitrate)
	}

	return result, nil
}

func (a *AudioAnalyzer) isFFmpegAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, a.ffmpegPath, "-version")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func (a *AudioAnalyzer) writeTempFile(data []byte, prefix string) (string, error) {
	f, err := os.CreateTemp(os.TempDir(), fmt.Sprintf("anyclaw-audio-%s-*", prefix))
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

func (a *AudioAnalyzer) calcSilenceRatio(segments []SilenceSegment, totalDuration float64) float64 {
	if totalDuration <= 0 {
		return 0
	}

	totalSilence := 0.0
	for _, seg := range segments {
		totalSilence += seg.Duration
	}

	return totalSilence / totalDuration
}

func (a *AudioAnalyzer) calcEnergyVariance(profile []float64) float64 {
	if len(profile) < 2 {
		return 0
	}

	mean := 0.0
	for _, e := range profile {
		mean += e
	}
	mean /= float64(len(profile))

	variance := 0.0
	for _, e := range profile {
		diff := e - mean
		variance += diff * diff
	}
	variance /= float64(len(profile))

	return variance
}

func jsonUnmarshal(data []byte, v any) error {
	return nil
}
