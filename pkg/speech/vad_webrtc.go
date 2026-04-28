package speech

import (
	"encoding/binary"
	"fmt"

	webrtcvad "github.com/godeps/webrtcvad-go"
)

type WebRTCVAD struct {
	inner      *VAD
	detector   *webrtcvad.VAD
	mode       int
	sampleRate int
	frameSize  int
	scratch    []byte
}

func NewWebRTCVAD(cfg VADConfig) (*WebRTCVAD, error) {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.FrameSize == 0 {
		cfg.FrameSize = 320
	}
	if cfg.Aggressiveness < 0 || cfg.Aggressiveness > 3 {
		cfg.Aggressiveness = 2
	}

	if !webrtcvad.ValidRateAndFrameLength(cfg.SampleRate, cfg.FrameSize) {
		return nil, fmt.Errorf("vad: invalid WebRTC sampleRate/frameSize combination: %d/%d", cfg.SampleRate, cfg.FrameSize)
	}

	detector, err := webrtcvad.New(cfg.Aggressiveness)
	if err != nil {
		return nil, fmt.Errorf("vad: failed to create WebRTC VAD: %w", err)
	}

	return &WebRTCVAD{
		inner:      NewVAD(cfg),
		detector:   detector,
		mode:       cfg.Aggressiveness,
		sampleRate: cfg.SampleRate,
		frameSize:  cfg.FrameSize,
		scratch:    make([]byte, cfg.FrameSize*2),
	}, nil
}

func (v *WebRTCVAD) Name() string {
	return "webrtc-vad"
}

func (v *WebRTCVAD) Type() VADProviderType {
	return VADProviderWebRTC
}

func (v *WebRTCVAD) ProcessFrame(samples []int16) VADState {
	if len(samples) == 0 {
		return v.inner.ProcessFrame(samples)
	}

	v.inner.mu.Lock()
	audio := v.frameBytes(samples)
	isSpeech, err := v.detector.IsSpeech(audio, v.sampleRate)
	if err != nil {
		v.inner.mu.Unlock()
		return v.inner.ProcessFrame(samples)
	}

	defer v.inner.mu.Unlock()

	energy := v.inner.calculateRMS(samples)
	zcr := v.inner.calculateZeroCrossingRate(samples)

	if isSpeech {
		v.inner.consecutiveSpeech++
		v.inner.consecutiveSilence = 0
	} else {
		v.inner.consecutiveSilence++
		v.inner.consecutiveSpeech = 0
	}

	switch v.inner.state {
	case VADStateSilence:
		if isSpeech {
			if v.inner.consecutiveSpeech >= v.inner.cfg.SpeechMinFrames {
				v.inner.state = VADStateSpeech
				v.inner.notifyListeners(VADStateSpeech, energy, zcr)
			}
		} else {
			v.inner.consecutiveSpeech = 0
		}

	case VADStateSpeech:
		if isSpeech {
			v.inner.consecutiveSilence = 0
		} else {
			if v.inner.consecutiveSilence >= v.inner.cfg.HangoverFrames {
				v.inner.state = VADStateSilence
				v.inner.consecutiveSpeech = 0
				v.inner.consecutiveSilence = 0
				v.inner.notifyListeners(VADStateSilence, energy, zcr)
			}
		}
	}

	return v.inner.state
}

func (v *WebRTCVAD) ProcessFloatFrame(samples []float32) VADState {
	return v.ProcessFrame(Float32ToInt16(samples))
}

func (v *WebRTCVAD) RegisterListener(listener VADStateListener) {
	v.inner.RegisterListener(listener)
}

func (v *WebRTCVAD) State() VADState {
	return v.inner.State()
}

func (v *WebRTCVAD) Reset() {
	v.inner.Reset()
}

func (v *WebRTCVAD) UpdateConfig(cfg VADConfig) {
	v.inner.UpdateConfig(cfg)
	if cfg.Aggressiveness >= 0 && cfg.Aggressiveness <= 3 {
		_ = v.detector.SetMode(cfg.Aggressiveness)
		v.mode = cfg.Aggressiveness
	}
}

func (v *WebRTCVAD) Config() VADConfig {
	cfg := v.inner.Config()
	cfg.Aggressiveness = v.mode
	return cfg
}

func (v *WebRTCVAD) frameBytes(samples []int16) []byte {
	size := len(samples) * 2
	if cap(v.scratch) < size {
		v.scratch = make([]byte, size)
	}
	out := v.scratch[:size]
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}

