package speech

import (
	"math"
	"sync"
)

type VADState string

const (
	VADStateSilence VADState = "silence"
	VADStateSpeech  VADState = "speech"
)

type VADConfig struct {
	SampleRate         int
	FrameSize          int
	EnergyThreshold    float64
	ZeroCrossThreshold int
	SpeechMinFrames    int
	SilenceFrames      int
	HangoverFrames     int
}

func DefaultVADConfig() VADConfig {
	return VADConfig{
		SampleRate:         16000,
		FrameSize:          320,
		EnergyThreshold:    0.01,
		ZeroCrossThreshold: 50,
		SpeechMinFrames:    3,
		SilenceFrames:      30,
		HangoverFrames:     10,
	}
}

type VAD struct {
	mu                 sync.Mutex
	cfg                VADConfig
	state              VADState
	consecutiveSpeech  int
	consecutiveSilence int
	listeners          []VADStateListener
}

type VADStateListener func(state VADState, energy float64, zcr float64)

func NewVAD(cfg VADConfig) *VAD {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.FrameSize == 0 {
		cfg.FrameSize = 320
	}
	if cfg.EnergyThreshold == 0 {
		cfg.EnergyThreshold = 0.01
	}
	if cfg.ZeroCrossThreshold == 0 {
		cfg.ZeroCrossThreshold = 50
	}
	if cfg.SpeechMinFrames == 0 {
		cfg.SpeechMinFrames = 3
	}
	if cfg.SilenceFrames == 0 {
		cfg.SilenceFrames = 30
	}
	if cfg.HangoverFrames == 0 {
		cfg.HangoverFrames = 10
	}

	return &VAD{
		cfg:   cfg,
		state: VADStateSilence,
	}
}

func (v *VAD) RegisterListener(listener VADStateListener) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.listeners = append(v.listeners, listener)
}

func (v *VAD) ProcessFrame(samples []int16) VADState {
	v.mu.Lock()
	defer v.mu.Unlock()

	energy := v.calculateRMS(samples)
	zcr := v.calculateZeroCrossingRate(samples)

	isSpeech := v.isSpeechFrame(energy, zcr)

	if isSpeech {
		v.consecutiveSpeech++
		v.consecutiveSilence = 0
	} else {
		v.consecutiveSilence++
		v.consecutiveSpeech = 0
	}

	switch v.state {
	case VADStateSilence:
		if isSpeech {
			if v.consecutiveSpeech >= v.cfg.SpeechMinFrames {
				v.state = VADStateSpeech
				v.notifyListeners(VADStateSpeech, energy, zcr)
			}
		} else {
			v.consecutiveSpeech = 0
		}

	case VADStateSpeech:
		if isSpeech {
			v.consecutiveSilence = 0
		} else {
			if v.consecutiveSilence >= v.cfg.HangoverFrames {
				v.state = VADStateSilence
				v.consecutiveSpeech = 0
				v.consecutiveSilence = 0
				v.notifyListeners(VADStateSilence, energy, zcr)
			}
		}
	}

	return v.state
}

func (v *VAD) ProcessFloatFrame(samples []float32) VADState {
	intSamples := make([]int16, len(samples))
	for i, s := range samples {
		clamped := s
		if clamped > 1.0 {
			clamped = 1.0
		}
		if clamped < -1.0 {
			clamped = -1.0
		}
		intSamples[i] = int16(clamped * 32767.0)
	}
	return v.ProcessFrame(intSamples)
}

func (v *VAD) isSpeechFrame(energy float64, zcr float64) bool {
	return energy > v.cfg.EnergyThreshold || zcr > float64(v.cfg.ZeroCrossThreshold)
}

func (v *VAD) calculateRMS(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sumSquares float64
	for _, s := range samples {
		normalized := float64(s) / 32768.0
		sumSquares += normalized * normalized
	}

	return math.Sqrt(sumSquares / float64(len(samples)))
}

func (v *VAD) calculateZeroCrossingRate(samples []int16) float64 {
	if len(samples) < 2 {
		return 0
	}

	var crossings int
	for i := 1; i < len(samples); i++ {
		if (samples[i] >= 0 && samples[i-1] < 0) || (samples[i] < 0 && samples[i-1] >= 0) {
			crossings++
		}
	}

	return float64(crossings)
}

func (v *VAD) State() VADState {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.state
}

func (v *VAD) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.state = VADStateSilence
	v.consecutiveSpeech = 0
	v.consecutiveSilence = 0
}

func (v *VAD) notifyListeners(state VADState, energy float64, zcr float64) {
	for _, listener := range v.listeners {
		listener(state, energy, zcr)
	}
}

func (v *VAD) UpdateConfig(cfg VADConfig) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if cfg.EnergyThreshold > 0 {
		v.cfg.EnergyThreshold = cfg.EnergyThreshold
	}
	if cfg.ZeroCrossThreshold > 0 {
		v.cfg.ZeroCrossThreshold = cfg.ZeroCrossThreshold
	}
	if cfg.SpeechMinFrames > 0 {
		v.cfg.SpeechMinFrames = cfg.SpeechMinFrames
	}
	if cfg.SilenceFrames > 0 {
		v.cfg.SilenceFrames = cfg.SilenceFrames
	}
	if cfg.HangoverFrames > 0 {
		v.cfg.HangoverFrames = cfg.HangoverFrames
	}
}

func (v *VAD) Config() VADConfig {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cfg
}

func NormalizeAudio(samples []int16) []float64 {
	result := make([]float64, len(samples))
	for i, s := range samples {
		result[i] = float64(s) / 32768.0
	}
	return result
}

func Float32ToInt16(samples []float32) []int16 {
	result := make([]int16, len(samples))
	for i, s := range samples {
		clamped := s
		if clamped > 1.0 {
			clamped = 1.0
		}
		if clamped < -1.0 {
			clamped = -1.0
		}
		result[i] = int16(clamped * 32767.0)
	}
	return result
}

func Int16ToWAV(samples []int16, sampleRate int, channels int) []byte {
	if len(samples) == 0 {
		return nil
	}

	bitsPerSample := 16
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := len(samples) * 2
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)

	copy(buf[0:4], []byte("RIFF"))
	buf[4] = byte(fileSize)
	buf[5] = byte(fileSize >> 8)
	buf[6] = byte(fileSize >> 16)
	buf[7] = byte(fileSize >> 24)

	copy(buf[8:12], []byte("WAVE"))

	copy(buf[12:16], []byte("fmt "))
	buf[16] = 16
	buf[17] = 0
	buf[18] = 0
	buf[19] = 0

	buf[20] = 1
	buf[21] = 0

	buf[22] = byte(channels)
	buf[23] = 0

	buf[24] = byte(sampleRate)
	buf[25] = byte(sampleRate >> 8)
	buf[26] = byte(sampleRate >> 16)
	buf[27] = byte(sampleRate >> 24)

	buf[28] = byte(byteRate)
	buf[29] = byte(byteRate >> 8)
	buf[30] = byte(byteRate >> 16)
	buf[31] = byte(byteRate >> 24)

	buf[32] = byte(blockAlign)
	buf[33] = 0

	buf[34] = byte(bitsPerSample)
	buf[35] = 0

	copy(buf[36:40], []byte("data"))
	buf[40] = byte(dataSize)
	buf[41] = byte(dataSize >> 8)
	buf[42] = byte(dataSize >> 16)
	buf[43] = byte(dataSize >> 24)

	for i, s := range samples {
		offset := 44 + i*2
		buf[offset] = byte(s)
		buf[offset+1] = byte(s >> 8)
	}

	return buf
}
