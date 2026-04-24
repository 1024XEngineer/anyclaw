package speech

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type SnowboyEngine struct {
	mu           sync.Mutex
	cfg          WakeWordEngineConfig
	pythonPath   string
	scriptPath   string
	modelPaths   []string
	commonModel  string
	isInit       bool
	frameSize    int
	sampleRate   int
	hotwordNames []string
}

func NewSnowboyEngine(cfg WakeWordEngineConfig) *SnowboyEngine {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.FrameSize == 0 {
		cfg.FrameSize = 512
	}
	if cfg.Sensitivity <= 0 {
		cfg.Sensitivity = 0.5
	}
	if cfg.ResourcePath == "" {
		cfg.ResourcePath = "lib/snowboy/resources/common.res"
	}
	return &SnowboyEngine{
		cfg:        cfg,
		frameSize:  cfg.FrameSize,
		sampleRate: cfg.SampleRate,
	}
}

func (s *SnowboyEngine) Name() string {
	return "snowboy"
}

func (s *SnowboyEngine) Type() WakeWordEngineType {
	return EngineSnowboy
}

func (s *SnowboyEngine) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isInit {
		return nil
	}

	if len(s.cfg.Keywords) == 0 && len(s.cfg.KeywordPaths) == 0 {
		return fmt.Errorf("snowboy: at least one keyword or keyword path required")
	}

	pythonPath := s.findPython()
	if pythonPath == "" {
		return fmt.Errorf("snowboy: python3 not found, required for snowboy wrapper")
	}
	s.pythonPath = pythonPath

	scriptPath := s.findScript()
	if scriptPath == "" {
		return fmt.Errorf("snowboy: demo script not found, install snowboy python demo")
	}
	s.scriptPath = scriptPath

	modelPaths := s.cfg.KeywordPaths
	if len(modelPaths) == 0 {
		modelPaths = make([]string, len(s.cfg.Keywords))
		for i, kw := range s.cfg.Keywords {
			modelPaths[i] = filepath.Join("lib", "snowboy", "models", kw+".pmdl")
		}
	}

	for _, path := range modelPaths {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("snowboy: model file not found: %s", path)
		}
	}

	commonModel := s.cfg.ResourcePath
	if commonModel != "" {
		if _, err := os.Stat(commonModel); err != nil {
			return fmt.Errorf("snowboy: common resource file not found: %s", commonModel)
		}
	}

	s.modelPaths = modelPaths
	s.commonModel = commonModel
	s.hotwordNames = make([]string, len(modelPaths))
	for i, path := range modelPaths {
		s.hotwordNames[i] = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if len(s.cfg.Keywords) > 0 {
		copy(s.hotwordNames, s.cfg.Keywords)
	}

	s.isInit = true
	return nil
}

func (s *SnowboyEngine) findPython() string {
	candidates := []string{"python3", "python"}
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func (s *SnowboyEngine) findScript() string {
	candidates := []string{
		"lib/snowboy/demo.py",
		"lib/snowboy/snowboydecoder.py",
		"snowboy_demo.py",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}
	return ""
}

func (s *SnowboyEngine) ProcessFrame(samples []int16) (*WakeWordDetectionResult, bool) {
	s.mu.Lock()
	if !s.isInit {
		s.mu.Unlock()
		return nil, false
	}
	s.mu.Unlock()

	if len(samples) == 0 {
		return nil, false
	}

	return s.processSamples(samples)
}

func (s *SnowboyEngine) processSamples(samples []int16) (*WakeWordDetectionResult, bool) {
	args := []string{
		s.scriptPath,
		"--sample_rate", fmt.Sprintf("%d", s.sampleRate),
		"--sensitivity", fmt.Sprintf("%.2f", s.cfg.Sensitivity),
	}

	if s.commonModel != "" {
		args = append(args, "--common_model", s.commonModel)
	}

	for _, modelPath := range s.modelPaths {
		args = append(args, "--model", modelPath)
	}

	args = append(args, "--stdin")

	cmd := exec.Command(s.pythonPath, args...)

	var stdinBuf bytes.Buffer
	for _, sample := range samples {
		lo := byte(sample)
		hi := byte(sample >> 8)
		stdinBuf.WriteByte(lo)
		stdinBuf.WriteByte(hi)
	}
	cmd.Stdin = &stdinBuf

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return nil, false
	}

	return s.parseOutput(stdoutBuf.String())
}

func (s *SnowboyEngine) parseOutput(output string) (*WakeWordDetectionResult, bool) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		if strings.Contains(lower, "detected") || strings.Contains(lower, "hotword") || strings.Contains(lower, "keyword") {
			for _, name := range s.hotwordNames {
				if strings.Contains(lower, strings.ToLower(name)) {
					return &WakeWordDetectionResult{
						Keyword:    name,
						Confidence: s.cfg.Sensitivity,
						Engine:     EngineSnowboy,
					}, true
				}
			}

			if len(s.hotwordNames) > 0 {
				return &WakeWordDetectionResult{
					Keyword:    s.hotwordNames[0],
					Confidence: s.cfg.Sensitivity,
					Engine:     EngineSnowboy,
				}, true
			}
		}
	}
	return nil, false
}

func (s *SnowboyEngine) ProcessBatch(audioPath string) (*WakeWordDetectionResult, bool) {
	s.mu.Lock()
	if !s.isInit {
		s.mu.Unlock()
		return nil, false
	}
	s.mu.Unlock()

	args := []string{
		s.scriptPath,
		"--sample_rate", fmt.Sprintf("%d", s.sampleRate),
		"--sensitivity", fmt.Sprintf("%.2f", s.cfg.Sensitivity),
		"--input_audio", audioPath,
	}

	if s.commonModel != "" {
		args = append(args, "--common_model", s.commonModel)
	}

	for _, modelPath := range s.modelPaths {
		args = append(args, "--model", modelPath)
	}

	cmd := exec.Command(s.pythonPath, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return nil, false
	}

	return s.parseOutput(stdoutBuf.String())
}

func (s *SnowboyEngine) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isInit = false
	return nil
}

func (s *SnowboyEngine) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}

func (s *SnowboyEngine) FrameLength() int {
	return s.frameSize
}

func (s *SnowboyEngine) SampleRate() int {
	return s.sampleRate
}

func (s *SnowboyEngine) IsInitialized() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isInit
}

func (s *SnowboyEngine) ModelPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.modelPaths...)
}

func (s *SnowboyEngine) HotwordNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.hotwordNames...)
}

func (s *SnowboyEngine) CommonModel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commonModel
}
