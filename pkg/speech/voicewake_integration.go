package speech

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
)

type VoiceWakeIntegration struct {
	mu             sync.Mutex
	cfg            VoiceWakeIntegrationConfig
	voiceWake      *VoiceWake
	processor      *VoiceWakeProcessor
	sessionMgr     *VoiceWakeSessionManager
	sessionTracker *VoiceWakeSessionTracker
	audioPlayer    AudioPlayer
	coordinator    *VoiceWakeCoordinator
	isRunning      bool
	startTime      time.Time
	stats          VoiceWakeIntegrationStats
}

type VoiceWakeIntegrationConfig struct {
	VoiceWakeConfig    VoiceWakeConfig
	ProcessorConfig    VoiceWakeProcessorConfig
	SessionConfig      VoiceWakeSessionManagerConfig
	AudioPlayerConfig  LocalAudioPlayerConfig
	CoordinatorConfig  VoiceWakeCoordinatorConfig
	AutoStart          bool
	SessionID          string
	EnableSessionTrack bool
	EnableCoordination bool
}

func DefaultVoiceWakeIntegrationConfig() VoiceWakeIntegrationConfig {
	return VoiceWakeIntegrationConfig{
		VoiceWakeConfig:    DefaultVoiceWakeConfig(),
		ProcessorConfig:    DefaultVoiceWakeProcessorConfig(),
		SessionConfig:      DefaultVoiceWakeSessionManagerConfig(),
		AudioPlayerConfig:  DefaultLocalAudioPlayerConfig(),
		CoordinatorConfig:  DefaultVoiceWakeCoordinatorConfig(),
		AutoStart:          false,
		SessionID:          "voicewake-default",
		EnableSessionTrack: true,
		EnableCoordination: false,
	}
}

type VoiceWakeIntegrationStats struct {
	WakeCount        int
	TranscriptCount  int
	ResponseCount    int
	AudioPlayedCount int
	ErrorCount       int
	TotalListenTime  time.Duration
	TotalSpeakTime   time.Duration
	LastWakeTime     time.Time
	LastResponseTime time.Time
}

func NewVoiceWakeIntegration(cfg VoiceWakeIntegrationConfig) *VoiceWakeIntegration {
	if cfg.SessionID == "" {
		cfg.SessionID = "voicewake-default"
	}

	return &VoiceWakeIntegration{
		cfg:   cfg,
		stats: VoiceWakeIntegrationStats{},
	}
}

func (i *VoiceWakeIntegration) Init(agent AgentRunner, ttsProcessor TTSProcessor) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.isRunning {
		return fmt.Errorf("voicewake-integration: already initialized")
	}

	i.sessionMgr = NewVoiceWakeSessionManager(i.cfg.SessionConfig)
	i.sessionTracker = NewVoiceWakeSessionTracker(i.sessionMgr)

	i.audioPlayer = NewLocalAudioPlayer(i.cfg.AudioPlayerConfig)

	i.cfg.ProcessorConfig.SessionID = i.cfg.SessionID
	i.cfg.ProcessorConfig.AutoSpeak = true

	i.voiceWake = NewVoiceWake(i.cfg.VoiceWakeConfig)

	i.cfg.ProcessorConfig.VoiceWake = i.voiceWake
	i.processor = NewVoiceWakeProcessor(i.cfg.ProcessorConfig)
	i.processor.SetAgent(agent)
	i.processor.SetTTSProcessor(ttsProcessor)
	i.processor.SetAudioPlayer(i.audioPlayer)

	i.processor.cfg.OnResponse = func(text string) {
		i.mu.Lock()
		i.stats.ResponseCount++
		i.stats.LastResponseTime = time.Now()
		i.mu.Unlock()

		i.recordResponse(text)
	}

	i.processor.cfg.OnAudioComplete = func(dur time.Duration) {
		i.mu.Lock()
		i.stats.AudioPlayedCount++
		i.stats.TotalSpeakTime += dur
		i.mu.Unlock()
	}

	i.processor.cfg.OnError = func(err error) {
		i.mu.Lock()
		i.stats.ErrorCount++
		i.mu.Unlock()

		log.Printf("voicewake-integration: processor error: %v", err)
	}

	i.voiceWake.RegisterListener(i.onVoiceWakeEvent)

	if i.cfg.EnableCoordination {
		i.coordinator = NewVoiceWakeCoordinator(i.cfg.CoordinatorConfig)
		i.coordinator.RegisterListener(i.onCoordinatorEvent)
	}

	_ = i.sessionMgr.GetOrCreateSession(i.cfg.SessionID)

	return nil
}

func (i *VoiceWakeIntegration) Start(ctx context.Context) error {
	i.mu.Lock()
	if i.isRunning {
		i.mu.Unlock()
		return fmt.Errorf("voicewake-integration: already running")
	}
	i.isRunning = true
	i.startTime = time.Now()
	coordinator := i.coordinator
	i.mu.Unlock()

	if err := i.processor.Start(); err != nil {
		i.mu.Lock()
		i.isRunning = false
		i.mu.Unlock()
		return fmt.Errorf("voicewake-integration: failed to start processor: %w", err)
	}

	if _, err := i.sessionTracker.StartSession(i.cfg.SessionID); err != nil {
		log.Printf("voicewake-integration: failed to start session: %v", err)
	}

	if coordinator != nil {
		if err := coordinator.Start(ctx); err != nil {
			log.Printf("voicewake-integration: failed to start coordinator: %v", err)
		}
	}

	if err := i.voiceWake.Start(ctx); err != nil {
		i.mu.Lock()
		i.isRunning = false
		i.mu.Unlock()
		return fmt.Errorf("voicewake-integration: failed to start voicewake: %w", err)
	}

	log.Printf("voicewake-integration: started (session: %s, coordination: %v)", i.cfg.SessionID, i.cfg.EnableCoordination)

	return nil
}

func (i *VoiceWakeIntegration) Stop() error {
	i.mu.Lock()
	if !i.isRunning {
		i.mu.Unlock()
		return nil
	}
	i.isRunning = false
	coordinator := i.coordinator
	i.mu.Unlock()

	if err := i.voiceWake.Stop(); err != nil {
		log.Printf("voicewake-integration: error stopping voicewake: %v", err)
	}

	if coordinator != nil {
		if err := coordinator.Stop(); err != nil {
			log.Printf("voicewake-integration: error stopping coordinator: %v", err)
		}
	}

	if err := i.sessionTracker.EndSession(i.cfg.SessionID); err != nil {
		log.Printf("voicewake-integration: error ending session: %v", err)
	}

	i.mu.Lock()
	i.stats.TotalListenTime = time.Since(i.startTime)
	i.mu.Unlock()

	log.Printf("voicewake-integration: stopped (stats: %+v)", i.Stats())

	return nil
}

func (i *VoiceWakeIntegration) onVoiceWakeEvent(event VoiceWakeEvent) {
	i.mu.Lock()
	switch event.Type {
	case VoiceWakeEventWakeDetected:
		i.stats.WakeCount++
		i.stats.LastWakeTime = time.Now()

	case VoiceWakeEventSpeechStart:
		// VAD detected speech start

	case VoiceWakeEventSpeechEnd:
		// VAD detected speech end

	case VoiceWakeEventError:
		i.stats.ErrorCount++
	}
	i.mu.Unlock()

	if event.Type == VoiceWakeEventWakeDetected {
		i.recordWakeEvent(event)

		i.mu.Lock()
		coordinator := i.coordinator
		i.mu.Unlock()

		if coordinator != nil && !coordinator.IsSuppressed() {
			phrase, _ := event.Data["phrase"].(string)
			confidence, _ := event.Data["confidence"].(float64)
			engine, _ := event.Data["engine"].(string)
			energy, _ := event.Data["energy"].(float64)

			allowed := coordinator.SubmitWake(phrase, confidence, energy, engine)
			if !allowed {
				log.Printf("voicewake-integration: wake suppressed by coordinator")
				return
			}
		}
	}
}

func (i *VoiceWakeIntegration) onCoordinatorEvent(event CoordinatorEvent) {
	switch event.Type {
	case CoordinatorEventArbitrationWon:
		log.Printf("coordinator: local device won arbitration")

	case CoordinatorEventArbitrationLost:
		winnerName, _ := event.Data["winner_name"].(string)
		log.Printf("coordinator: lost arbitration to %s", winnerName)

	case CoordinatorEventSuppressed:
		deviceName, _ := event.Data["device_name"].(string)
		duration, _ := event.Data["duration"].(time.Duration)
		log.Printf("coordinator: suppressed for %v by %s", duration, deviceName)

	case CoordinatorEventDeviceDiscovered:
		deviceName, _ := event.Data["device_name"].(string)
		ipAddress, _ := event.Data["ip_address"].(string)
		log.Printf("coordinator: discovered device %s at %s", deviceName, ipAddress)

	case CoordinatorEventDeviceLost:
		deviceName, _ := event.Data["device_name"].(string)
		log.Printf("coordinator: lost device %s", deviceName)
	}
}

func (i *VoiceWakeIntegration) recordWakeEvent(event VoiceWakeEvent) {
	if i.sessionMgr == nil {
		return
	}

	phrase, _ := event.Data["phrase"].(string)
	confidence, _ := event.Data["confidence"].(float64)

	entry := WakeEventEntry{
		Phrase:     phrase,
		Confidence: confidence,
		Engine:     fmt.Sprintf("%v", event.Data["engine"]),
		Timestamp:  event.Timestamp,
	}

	_ = i.sessionMgr.AddWakeEvent(i.cfg.SessionID, entry)
}

func (i *VoiceWakeIntegration) recordTranscript(text string, confidence float64, language string, duration time.Duration) {
	if i.sessionMgr == nil {
		return
	}

	entry := TranscriptEntry{
		Text:       text,
		Confidence: confidence,
		Language:   language,
		Duration:   duration,
		Timestamp:  time.Now(),
	}

	i.mu.Lock()
	i.stats.TranscriptCount++
	i.mu.Unlock()

	_ = i.sessionMgr.AddTranscript(i.cfg.SessionID, entry)
}

func (i *VoiceWakeIntegration) recordResponse(text string) {
	if i.sessionMgr == nil {
		return
	}

	entry := ResponseEntry{
		Text:      text,
		Timestamp: time.Now(),
		IsSpoken:  i.processor.cfg.AutoSpeak,
	}

	_ = i.sessionMgr.AddResponse(i.cfg.SessionID, entry)
}

func (i *VoiceWakeIntegration) AddToHistory(role string, content string) error {
	if i.sessionMgr == nil {
		return nil
	}

	msg := prompt.Message{
		Role:    role,
		Content: content,
	}

	return i.sessionMgr.AddToHistory(i.cfg.SessionID, msg)
}

func (i *VoiceWakeIntegration) GetHistory() ([]prompt.Message, error) {
	if i.sessionMgr == nil {
		return nil, nil
	}

	return i.sessionMgr.GetHistory(i.cfg.SessionID)
}

func (i *VoiceWakeIntegration) SetHistory(history []prompt.Message) error {
	if i.sessionMgr == nil {
		return nil
	}

	return i.sessionMgr.SetHistory(i.cfg.SessionID, history)
}

func (i *VoiceWakeIntegration) Stats() VoiceWakeIntegrationStats {
	i.mu.Lock()
	defer i.mu.Unlock()

	stats := i.stats
	if i.isRunning {
		stats.TotalListenTime += time.Since(i.startTime)
	}
	return stats
}

func (i *VoiceWakeIntegration) IsRunning() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.isRunning
}

func (i *VoiceWakeIntegration) IsProcessing() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.processor == nil {
		return false
	}
	return i.processor.IsProcessing()
}

func (i *VoiceWakeIntegration) CancelConversation() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.processor != nil {
		i.processor.CancelConversation()
	}
}

func (i *VoiceWakeIntegration) VoiceWake() *VoiceWake {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.voiceWake
}

func (i *VoiceWakeIntegration) Processor() *VoiceWakeProcessor {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.processor
}

func (i *VoiceWakeIntegration) SessionManager() *VoiceWakeSessionManager {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.sessionMgr
}

func (i *VoiceWakeIntegration) AudioPlayer() AudioPlayer {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.audioPlayer
}

func (i *VoiceWakeIntegration) SetAgent(agent AgentRunner) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.processor != nil {
		i.processor.SetAgent(agent)
	}
}

func (i *VoiceWakeIntegration) SetTTSProcessor(processor TTSProcessor) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.processor != nil {
		i.processor.SetTTSProcessor(processor)
	}
}

func (i *VoiceWakeIntegration) SetSessionID(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cfg.SessionID = id
	if i.processor != nil {
		i.processor.cfg.SessionID = id
	}
}

func (i *VoiceWakeIntegration) SetAutoSpeak(enabled bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.processor != nil {
		i.processor.cfg.AutoSpeak = enabled
	}
}

func (i *VoiceWakeIntegration) SetSensitivity(s float64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.voiceWake != nil {
		if wd := i.voiceWake.WakeDetector(); wd != nil {
			wd.SetSensitivity(s)
		}
	}
}

func (i *VoiceWakeIntegration) RegisterEngine(engineType WakeWordEngineType, cfg WakeWordEngineConfig) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.voiceWake == nil {
		return fmt.Errorf("voicewake-integration: not initialized")
	}

	return i.voiceWake.RegisterEngine(engineType, cfg)
}

func (i *VoiceWakeIntegration) RecentTranscripts(n int) ([]TranscriptEntry, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.sessionMgr == nil {
		return nil, nil
	}

	return i.sessionMgr.RecentTranscripts(i.cfg.SessionID, n)
}

func (i *VoiceWakeIntegration) RecentResponses(n int) ([]ResponseEntry, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.sessionMgr == nil {
		return nil, nil
	}

	return i.sessionMgr.RecentResponses(i.cfg.SessionID, n)
}

func (i *VoiceWakeIntegration) SessionStats() (map[string]any, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.sessionMgr == nil {
		return nil, nil
	}

	return i.sessionMgr.SessionStats(i.cfg.SessionID)
}

func (i *VoiceWakeIntegration) Coordinator() *VoiceWakeCoordinator {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.coordinator
}

func (i *VoiceWakeIntegration) EnableCoordination(enabled bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cfg.EnableCoordination = enabled
}

func (i *VoiceWakeIntegration) IsCoordinationEnabled() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.cfg.EnableCoordination
}

func (i *VoiceWakeIntegration) CoordinatorStats() CoordinatorStats {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator == nil {
		return CoordinatorStats{}
	}
	return i.coordinator.Stats()
}

func (i *VoiceWakeIntegration) SetDevicePriority(priority int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator != nil {
		i.coordinator.SetPriority(priority)
	}
}

func (i *VoiceWakeIntegration) SetElectionMode(mode ElectionMode) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator != nil {
		i.coordinator.SetElectionMode(mode)
	}
}

func (i *VoiceWakeIntegration) SetPreferLocal(prefer bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator != nil {
		i.coordinator.SetPreferLocal(prefer)
	}
}

func (i *VoiceWakeIntegration) DiscoveredDevices() []DeviceInfo {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator == nil {
		return nil
	}
	return i.coordinator.GetDevices()
}

func (i *VoiceWakeIntegration) IsSuppressed() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator == nil {
		return false
	}
	return i.coordinator.IsSuppressed()
}

func (i *VoiceWakeIntegration) SuppressionRemaining() time.Duration {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.coordinator == nil {
		return 0
	}
	return i.coordinator.SuppressionRemaining()
}
