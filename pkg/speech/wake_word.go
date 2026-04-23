package speech

import (
	"math"
	"strings"
	"sync"
)

type WakeWordDetector struct {
	mu          sync.Mutex
	wakeWords   []WakeWord
	sensitivity float64
	listeners   []WakeWordListener
}

type WakeWord struct {
	Phrase   string
	Aliases  []string
	Enabled  bool
	Callback func(matchedPhrase string)
}

type WakeWordListener func(phrase string, confidence float64)

type WakeWordConfig struct {
	WakeWords   []string
	Sensitivity float64
}

func DefaultWakeWordConfig() WakeWordConfig {
	return WakeWordConfig{
		WakeWords:   []string{"hey anyclaw", "hi anyclaw", "ok anyclaw"},
		Sensitivity: 0.7,
	}
}

func NewWakeWordDetector(cfg WakeWordConfig) *WakeWordDetector {
	detector := &WakeWordDetector{
		sensitivity: cfg.Sensitivity,
	}

	if detector.sensitivity <= 0 {
		detector.sensitivity = 0.7
	}

	for _, phrase := range cfg.WakeWords {
		detector.AddWakeWord(WakeWord{
			Phrase:  phrase,
			Enabled: true,
		})
	}

	return detector
}

func (d *WakeWordDetector) AddWakeWord(ww WakeWord) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if ww.Phrase == "" {
		return
	}

	if !ww.Enabled {
		ww.Enabled = true
	}

	d.wakeWords = append(d.wakeWords, ww)
}

func (d *WakeWordDetector) RemoveWakeWord(phrase string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	filtered := make([]WakeWord, 0, len(d.wakeWords))
	for _, ww := range d.wakeWords {
		if !strings.EqualFold(ww.Phrase, phrase) {
			filtered = append(filtered, ww)
		}
	}
	d.wakeWords = filtered
}

func (d *WakeWordDetector) RegisterListener(listener WakeWordListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, listener)
}

func (d *WakeWordDetector) Detect(transcript string) (string, float64, bool) {
	d.mu.Lock()
	wakeWords := make([]WakeWord, len(d.wakeWords))
	copy(wakeWords, d.wakeWords)
	listeners := make([]WakeWordListener, len(d.listeners))
	copy(listeners, d.listeners)
	sensitivity := d.sensitivity
	d.mu.Unlock()

	if transcript == "" {
		return "", 0, false
	}

	normalized := normalizeText(transcript)

	for _, ww := range wakeWords {
		if !ww.Enabled {
			continue
		}

		phrases := append([]string{ww.Phrase}, ww.Aliases...)
		for _, phrase := range phrases {
			confidence := matchPhrase(normalized, normalizeText(phrase))
			if confidence >= sensitivity {
				if ww.Callback != nil {
					ww.Callback(phrase)
				}

				for _, listener := range listeners {
					listener(phrase, confidence)
				}

				return phrase, confidence, true
			}
		}
	}

	return "", 0, false
}

func (d *WakeWordDetector) DetectStream(transcript string) (string, float64, bool) {
	return d.Detect(transcript)
}

func (d *WakeWordDetector) SetSensitivity(s float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if s >= 0 && s <= 1 {
		d.sensitivity = s
	}
}

func (d *WakeWordDetector) Sensitivity() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sensitivity
}

func (d *WakeWordDetector) WakeWords() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]string, 0, len(d.wakeWords))
	for _, ww := range d.wakeWords {
		if ww.Enabled {
			result = append(result, ww.Phrase)
		}
	}
	return result
}

func (d *WakeWordDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.wakeWords {
		d.wakeWords[i].Enabled = true
	}
}

func normalizeText(text string) string {
	text = strings.ToLower(text)
	text = strings.TrimSpace(text)

	replacements := map[string]string{
		"'":  "",
		"\"": "",
		".":  "",
		",":  "",
		"!":  "",
		"?":  "",
	}
	for old, new := range replacements {
		text = strings.ReplaceAll(text, old, new)
	}

	return strings.Join(strings.Fields(text), " ")
}

func matchPhrase(input, phrase string) float64 {
	if input == phrase {
		return 1.0
	}

	if strings.Contains(input, phrase) {
		return 0.9
	}

	if strings.HasPrefix(input, phrase) {
		return 0.85
	}

	if levenshteinDistance(input, phrase) <= 1 {
		return 0.8
	}

	inputWords := strings.Fields(input)
	phraseWords := strings.Fields(phrase)

	if len(phraseWords) == 0 {
		return 0
	}

	matchedWords := 0
	for _, pw := range phraseWords {
		for _, iw := range inputWords {
			if iw == pw || levenshteinDistance(iw, pw) <= 2 {
				matchedWords++
				break
			}
		}
	}

	wordMatchRatio := float64(matchedWords) / float64(len(phraseWords))

	if wordMatchRatio >= 0.5 {
		return wordMatchRatio * 0.75
	}

	return 0
}

func levenshteinDistance(s1, s2 string) int {
	s1Runes := []rune(s1)
	s2Runes := []rune(s2)
	len1 := len(s1Runes)
	len2 := len(s2Runes)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if s1Runes[i-1] == s2Runes[j-1] {
				cost = 0
			}

			matrix[i][j] = min3(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}

	return matrix[len1][len2]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func MFCC(samples []float64, sampleRate int, numCoefficients int, numFilters int) [][]float64 {
	if len(samples) == 0 || sampleRate <= 0 || numCoefficients <= 0 || numFilters <= 0 {
		return nil
	}

	frameSize := 256
	hopSize := frameSize / 2
	numFrames := (len(samples)-frameSize)/hopSize + 1

	if numFrames <= 0 {
		return nil
	}

	features := make([][]float64, numFrames)

	for i := 0; i < numFrames; i++ {
		start := i * hopSize
		end := start + frameSize
		if end > len(samples) {
			break
		}

		frame := make([]float64, frameSize)
		copy(frame, samples[start:end])

		frame = applyHammingWindow(frame)

		magSpectrum := computeMagnitudeSpectrum(frame)

		energies := applyMelFilterBank(magSpectrum, sampleRate, numFilters)

		logEnergies := make([]float64, numFilters)
		for j, e := range energies {
			if e > 0 {
				logEnergies[j] = math.Log(e + 1e-10)
			} else {
				logEnergies[j] = 0
			}
		}

		coefficients := computeDCT(logEnergies, numCoefficients)

		if len(coefficients) > 0 {
			coefficients[0] = 0
		}

		features[i] = coefficients
	}

	return features
}

func applyHammingWindow(frame []float64) []float64 {
	N := len(frame)
	result := make([]float64, N)
	for i := 0; i < N; i++ {
		window := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(N-1))
		result[i] = frame[i] * window
	}
	return result
}

func computeMagnitudeSpectrum(frame []float64) []float64 {
	N := len(frame)
	halfN := N / 2

	magnitude := make([]float64, halfN)

	for k := 0; k < halfN; k++ {
		var real, imag float64
		for n := 0; n < N; n++ {
			angle := 2 * math.Pi * float64(k) * float64(n) / float64(N)
			real += frame[n] * math.Cos(angle)
			imag -= frame[n] * math.Sin(angle)
		}
		magnitude[k] = math.Sqrt(real*real+imag*imag) / float64(N)
	}

	return magnitude
}

func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1+hz/700.0)
}

func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10, mel/2595.0) - 1)
}

func applyMelFilterBank(magnitude []float64, sampleRate int, numFilters int) []float64 {
	N := len(magnitude)
	if N == 0 {
		return nil
	}

	fMin := 0.0
	fMax := float64(sampleRate) / 2.0

	melMin := hzToMel(fMin)
	melMax := hzToMel(fMax)

	melPoints := make([]float64, numFilters+2)
	for i := 0; i < numFilters+2; i++ {
		mel := melMin + float64(i)*(melMax-melMin)/float64(numFilters+1)
		melPoints[i] = melToHz(mel)
	}

	binFreqs := make([]float64, numFilters+2)
	for i, mel := range melPoints {
		binFreqs[i] = float64(N) * mel / float64(sampleRate)
	}

	energies := make([]float64, numFilters)

	for m := 0; m < numFilters; m++ {
		var energy float64
		lower := int(binFreqs[m])
		center := int(binFreqs[m+1])
		upper := int(binFreqs[m+2])

		for k := lower; k < center && k < N; k++ {
			if k >= 0 {
				weight := float64(k-lower) / float64(center-lower)
				energy += magnitude[k] * magnitude[k] * weight
			}
		}

		for k := center; k < upper && k < N; k++ {
			if k >= 0 {
				weight := float64(upper-k) / float64(upper-center)
				energy += magnitude[k] * magnitude[k] * weight
			}
		}

		energies[m] = energy
	}

	return energies
}

func computeDCT(input []float64, numCoefficients int) []float64 {
	N := len(input)
	if N == 0 || numCoefficients <= 0 {
		return nil
	}

	if numCoefficients > N {
		numCoefficients = N
	}

	output := make([]float64, numCoefficients)

	for k := 0; k < numCoefficients; k++ {
		var sum float64
		for n := 0; n < N; n++ {
			sum += input[n] * math.Cos(math.Pi*float64(k)*(float64(n)+0.5)/float64(N))
		}
		output[k] = sum
	}

	return output
}

func DTW(features1, features2 [][]float64) float64 {
	if len(features1) == 0 || len(features2) == 0 {
		return math.Inf(1)
	}

	n := len(features1)
	m := len(features2)

	if len(features1[0]) != len(features2[0]) {
		return math.Inf(1)
	}

	dt := make([][]float64, n+1)
	for i := range dt {
		dt[i] = make([]float64, m+1)
		for j := range dt[i] {
			dt[i][j] = math.Inf(1)
		}
	}
	dt[0][0] = 0

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := euclideanDistance(features1[i-1], features2[j-1])
			dt[i][j] = cost + min3Float(dt[i-1][j], dt[i][j-1], dt[i-1][j-1])
		}
	}

	return dt[n][m]
}

func euclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return math.Inf(1)
	}

	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

func min3Float(a, b, c float64) float64 {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
