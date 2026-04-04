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

type PorcupineEngine struct {
	mu            sync.Mutex
	cfg           WakeWordEngineConfig
	binaryPath    string
	keywordPaths  []string
	sensitivities []float64
	isInit        bool
	frameSize     int
	sampleRate    int
}

func NewPorcupineEngine(cfg WakeWordEngineConfig) *PorcupineEngine {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.FrameSize == 0 {
		cfg.FrameSize = 512
	}
	if cfg.Sensitivity <= 0 {
		cfg.Sensitivity = 0.5
	}
	return &PorcupineEngine{
		cfg:        cfg,
		frameSize:  cfg.FrameSize,
		sampleRate: cfg.SampleRate,
	}
}

func (p *PorcupineEngine) Name() string {
	return "porcupine"
}

func (p *PorcupineEngine) Type() WakeWordEngineType {
	return EnginePorcupine
}

func (p *PorcupineEngine) Init() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isInit {
		return nil
	}

	if len(p.cfg.Keywords) == 0 && len(p.cfg.KeywordPaths) == 0 {
		return fmt.Errorf("porcupine: at least one keyword or keyword path required")
	}

	if p.cfg.AccessKey == "" {
		return fmt.Errorf("porcupine: access_key required")
	}

	binaryPath := p.findBinary()
	if binaryPath == "" {
		return fmt.Errorf("porcupine: binary not found, install porcupine_demo or pv_porcupine_demo")
	}
	p.binaryPath = binaryPath

	keywordPaths := p.cfg.KeywordPaths
	if len(keywordPaths) == 0 {
		keywordPaths = make([]string, len(p.cfg.Keywords))
		for i, kw := range p.cfg.Keywords {
			keywordPaths[i] = filepath.Join("lib", "porcupine", "keywords", kw+"_en.ppn")
		}
	}

	for _, path := range keywordPaths {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("porcupine: keyword file not found: %s", path)
		}
	}

	if p.cfg.ModelPath != "" {
		if _, err := os.Stat(p.cfg.ModelPath); err != nil {
			return fmt.Errorf("porcupine: model file not found: %s", p.cfg.ModelPath)
		}
	}

	p.keywordPaths = keywordPaths
	p.sensitivities = make([]float64, len(keywordPaths))
	for i := range p.sensitivities {
		p.sensitivities[i] = p.cfg.Sensitivity
	}
	p.isInit = true
	return nil
}

func (p *PorcupineEngine) findBinary() string {
	candidates := []string{
		"porcupine_demo",
		"pv_porcupine_demo",
		"porcupine_demo_mic",
	}
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func (p *PorcupineEngine) ProcessFrame(samples []int16) (*WakeWordDetectionResult, bool) {
	p.mu.Lock()
	if !p.isInit {
		p.mu.Unlock()
		return nil, false
	}
	p.mu.Unlock()

	if len(samples) == 0 {
		return nil, false
	}

	return p.processSamples(samples)
}

func (p *PorcupineEngine) processSamples(samples []int16) (*WakeWordDetectionResult, bool) {
	args := p.buildArgs()

	cmd := exec.Command(p.binaryPath, args...)

	var stdinBuf bytes.Buffer
	for _, s := range samples {
		lo := byte(s)
		hi := byte(s >> 8)
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

	output := stdoutBuf.String()
	return p.parseOutput(output)
}

func (p *PorcupineEngine) buildArgs() []string {
	args := []string{
		"--access_key", p.cfg.AccessKey,
		"--sample_rate", fmt.Sprintf("%d", p.sampleRate),
	}

	if p.cfg.ModelPath != "" {
		args = append(args, "--model_path", p.cfg.ModelPath)
	}

	for i, kwPath := range p.keywordPaths {
		args = append(args, "--keyword_path", kwPath)
		args = append(args, "--sensitivity", fmt.Sprintf("%.2f", p.sensitivities[i]))
	}

	args = append(args, "--audio_device", "stdin")

	return args
}

func (p *PorcupineEngine) parseOutput(output string) (*WakeWordDetectionResult, bool) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "keyword_index") || strings.Contains(line, "detected") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(part, "index") && i+1 < len(parts) {
					idxStr := strings.Trim(parts[i+1], "[]:,")
					var idx int
					fmt.Sscanf(idxStr, "%d", &idx)
					if idx >= 0 && idx < len(p.keywordPaths) {
						keyword := p.keywordPaths[idx]
						if idx < len(p.cfg.Keywords) && p.cfg.Keywords[idx] != "" {
							keyword = p.cfg.Keywords[idx]
						}
						return &WakeWordDetectionResult{
							Keyword:    keyword,
							Confidence: p.sensitivities[idx],
							Engine:     EnginePorcupine,
						}, true
					}
				}
			}
		}
	}
	return nil, false
}

func (p *PorcupineEngine) ProcessBatch(audioPath string) (*WakeWordDetectionResult, bool) {
	p.mu.Lock()
	if !p.isInit {
		p.mu.Unlock()
		return nil, false
	}
	p.mu.Unlock()

	args := p.buildArgs()
	args = append(args, "--input_audio_path", audioPath)

	cmd := exec.Command(p.binaryPath, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return nil, false
	}

	return p.parseOutput(stdoutBuf.String())
}

func (p *PorcupineEngine) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isInit = false
	return nil
}

func (p *PorcupineEngine) Reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return nil
}

func (p *PorcupineEngine) FrameLength() int {
	return p.frameSize
}

func (p *PorcupineEngine) SampleRate() int {
	return p.sampleRate
}

func (p *PorcupineEngine) IsInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isInit
}

func (p *PorcupineEngine) KeywordPaths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.keywordPaths...)
}

func (p *PorcupineEngine) Sensitivities() []float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]float64(nil), p.sensitivities...)
}
