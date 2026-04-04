package speech

import (
	"context"
	"fmt"
	"sync"
)

type WakeWordEngineType string

const (
	EngineBuiltIn   WakeWordEngineType = "builtin"
	EnginePorcupine WakeWordEngineType = "porcupine"
	EngineSnowboy   WakeWordEngineType = "snowboy"
)

type WakeWordDetectionResult struct {
	Keyword    string
	Confidence float64
	Engine     WakeWordEngineType
}

type WakeWordEngine interface {
	Name() string
	Type() WakeWordEngineType
	Init() error
	ProcessFrame(samples []int16) (*WakeWordDetectionResult, bool)
	Close() error
	Reset() error
}

type WakeWordEngineFactory func(cfg WakeWordEngineConfig) (WakeWordEngine, error)

type WakeWordEngineConfig struct {
	Type         WakeWordEngineType
	Keywords     []string
	KeywordPaths []string
	Sensitivity  float64
	LibraryPath  string
	ModelPath    string
	ResourcePath string
	AccessKey    string
	SampleRate   int
	FrameSize    int
	Extra        map[string]string
}

type WakeWordEngineRouter struct {
	mu        sync.Mutex
	engines   map[string]WakeWordEngine
	active    string
	factory   map[WakeWordEngineType]WakeWordEngineFactory
	cfg       WakeWordEngineConfig
	listeners []WakeWordListener
}

func NewWakeWordEngineRouter(cfg WakeWordEngineConfig) *WakeWordEngineRouter {
	return &WakeWordEngineRouter{
		engines: make(map[string]WakeWordEngine),
		factory: make(map[WakeWordEngineType]WakeWordEngineFactory),
		cfg:     cfg,
	}
}

func (r *WakeWordEngineRouter) RegisterFactory(engineType WakeWordEngineType, factory WakeWordEngineFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factory[engineType] = factory
}

func (r *WakeWordEngineRouter) CreateEngine(engineType WakeWordEngineType, cfg WakeWordEngineConfig) error {
	r.mu.Lock()
	factory, ok := r.factory[engineType]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("voicewake: no factory registered for engine type %s", engineType)
	}

	engine, err := factory(cfg)
	if err != nil {
		return fmt.Errorf("voicewake: failed to create engine %s: %w", engineType, err)
	}

	if err := engine.Init(); err != nil {
		return fmt.Errorf("voicewake: failed to initialize engine %s: %w", engineType, err)
	}

	r.mu.Lock()
	name := engine.Name()
	r.engines[name] = engine
	if r.active == "" {
		r.active = name
	}
	r.mu.Unlock()

	return nil
}

func (r *WakeWordEngineRouter) ProcessFrame(samples []int16) (*WakeWordDetectionResult, bool) {
	r.mu.Lock()
	active := r.active
	engine := r.engines[active]
	r.mu.Unlock()

	if engine == nil {
		return nil, false
	}

	return engine.ProcessFrame(samples)
}

func (r *WakeWordEngineRouter) SetActive(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.engines[name]; !ok {
		return fmt.Errorf("voicewake: engine %s not found", name)
	}
	r.active = name
	return nil
}

func (r *WakeWordEngineRouter) ActiveEngine() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func (r *WakeWordEngineRouter) RegisterListener(listener WakeWordListener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, listener)
}

func (r *WakeWordEngineRouter) notifyListeners(phrase string, confidence float64) {
	r.mu.Lock()
	listeners := make([]WakeWordListener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	for _, listener := range listeners {
		listener(phrase, confidence)
	}
}

func (r *WakeWordEngineRouter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for name, engine := range r.engines {
		if err := engine.Close(); err != nil {
			lastErr = fmt.Errorf("voicewake: error closing engine %s: %w", name, err)
		}
		delete(r.engines, name)
	}
	r.active = ""
	return lastErr
}

func (r *WakeWordEngineRouter) Reset() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for name, engine := range r.engines {
		if err := engine.Reset(); err != nil {
			lastErr = fmt.Errorf("voicewake: error resetting engine %s: %w", name, err)
		}
	}
	return lastErr
}

func (r *WakeWordEngineRouter) Engines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	return names
}

type WakeWordEngineAdapter struct {
	router    *WakeWordEngineRouter
	detector  *WakeWordDetector
	mu        sync.Mutex
	useEngine bool
}

func NewWakeWordEngineAdapter(router *WakeWordEngineRouter, detector *WakeWordDetector) *WakeWordEngineAdapter {
	return &WakeWordEngineAdapter{
		router:    router,
		detector:  detector,
		useEngine: router != nil && len(router.Engines()) > 0,
	}
}

func (a *WakeWordEngineAdapter) ProcessFrame(samples []int16) (*WakeWordDetectionResult, bool) {
	a.mu.Lock()
	useEngine := a.useEngine
	router := a.router
	detector := a.detector
	a.mu.Unlock()

	if useEngine && router != nil {
		result, detected := router.ProcessFrame(samples)
		if detected && result != nil {
			router.notifyListeners(result.Keyword, result.Confidence)
		}
		return result, detected
	}

	if detector != nil {
		return nil, false
	}

	return nil, false
}

func (a *WakeWordEngineAdapter) DetectTranscript(transcript string) (string, float64, bool) {
	a.mu.Lock()
	detector := a.detector
	a.mu.Unlock()

	if detector != nil {
		return detector.Detect(transcript)
	}
	return "", 0, false
}

func (a *WakeWordEngineAdapter) SetUseEngine(use bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.useEngine = use
}

func (a *WakeWordEngineAdapter) UseEngine() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.useEngine
}

func (a *WakeWordEngineAdapter) RegisterListener(listener WakeWordListener) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.router != nil {
		a.router.RegisterListener(listener)
	}
	if a.detector != nil {
		a.detector.RegisterListener(listener)
	}
}

type MockWakeWordEngine struct {
	name         string
	keyword      string
	triggered    bool
	frameCount   int
	triggerAfter int
	sampleRate   int
	frameSize    int
}

func NewMockWakeWordEngine(keyword string, triggerAfter int) *MockWakeWordEngine {
	return &MockWakeWordEngine{
		name:         "mock-" + keyword,
		keyword:      keyword,
		triggerAfter: triggerAfter,
		sampleRate:   16000,
		frameSize:    320,
	}
}

func (m *MockWakeWordEngine) Name() string {
	return m.name
}

func (m *MockWakeWordEngine) Type() WakeWordEngineType {
	return "mock"
}

func (m *MockWakeWordEngine) Init() error {
	return nil
}

func (m *MockWakeWordEngine) ProcessFrame(samples []int16) (*WakeWordDetectionResult, bool) {
	m.frameCount++

	if m.frameCount >= m.triggerAfter {
		m.frameCount = 0
		return &WakeWordDetectionResult{
			Keyword:    m.keyword,
			Confidence: 0.95,
			Engine:     "mock",
		}, true
	}

	return nil, false
}

func (m *MockWakeWordEngine) Close() error {
	m.frameCount = 0
	return nil
}

func (m *MockWakeWordEngine) Reset() error {
	m.frameCount = 0
	m.triggered = false
	return nil
}

type WakeWordEngineContext struct {
	Ctx        context.Context
	SampleRate int
	Channels   int
	FrameSize  int
}
