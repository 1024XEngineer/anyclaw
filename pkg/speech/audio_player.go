package speech

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

type AudioPlayer interface {
	Play(ctx context.Context, audio []byte, format AudioFormat) error
	Stop() error
	IsPlaying() bool
}

type LocalAudioPlayer struct {
	mu         sync.Mutex
	isPlaying  bool
	currentCmd *exec.Cmd
	tempDir    string
	playerCmd  string
	volume     float64
}

type LocalAudioPlayerConfig struct {
	TempDir   string
	PlayerCmd string
	Volume    float64
}

func DefaultLocalAudioPlayerConfig() LocalAudioPlayerConfig {
	return LocalAudioPlayerConfig{
		Volume: 1.0,
	}
}

func NewLocalAudioPlayer(cfg LocalAudioPlayerConfig) *LocalAudioPlayer {
	if cfg.Volume <= 0 || cfg.Volume > 2.0 {
		cfg.Volume = 1.0
	}

	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}

	if cfg.PlayerCmd == "" {
		cfg.PlayerCmd = detectPlayer()
	}

	return &LocalAudioPlayer{
		tempDir:   cfg.TempDir,
		playerCmd: cfg.PlayerCmd,
		volume:    cfg.Volume,
	}
}

func detectPlayer() string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("afplay"); err == nil {
			return "afplay"
		}
	case "linux":
		players := []string{"aplay", "paplay", "ffplay", "vlc", "mplayer"}
		for _, p := range players {
			if _, err := exec.LookPath(p); err == nil {
				return p
			}
		}
	case "windows":
		if _, err := exec.LookPath("powershell"); err == nil {
			return "powershell"
		}
	}
	return ""
}

func (p *LocalAudioPlayer) Play(ctx context.Context, audio []byte, format AudioFormat) error {
	p.mu.Lock()
	if p.isPlaying {
		p.mu.Unlock()
		p.Stop()
		p.mu.Lock()
	}

	playerCmd := p.playerCmd
	if playerCmd == "" {
		p.mu.Unlock()
		return fmt.Errorf("audio-player: no player available, install aplay/afplay/ffplay")
	}
	p.mu.Unlock()

	tmpPath, err := p.writeTempFile(audio, format)
	if err != nil {
		return fmt.Errorf("audio-player: failed to write temp file: %w", err)
	}
	defer os.Remove(tmpPath)

	return p.playFile(ctx, tmpPath, format)
}

func (p *LocalAudioPlayer) writeTempFile(audio []byte, format AudioFormat) (string, error) {
	ext := string(format)
	if ext == "" {
		ext = "wav"
	}

	f, err := os.CreateTemp(p.tempDir, fmt.Sprintf("voicewake-*.%s", ext))
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.Write(audio); err != nil {
		return "", err
	}

	return f.Name(), nil
}

func (p *LocalAudioPlayer) playFile(ctx context.Context, filePath string, format AudioFormat) error {
	p.mu.Lock()
	p.isPlaying = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.isPlaying = false
		p.mu.Unlock()
	}()

	var cmd *exec.Cmd

	switch p.playerCmd {
	case "afplay":
		cmd = exec.CommandContext(ctx, "afplay", filePath)

	case "aplay":
		cmd = exec.CommandContext(ctx, "aplay", "-q", filePath)

	case "paplay":
		cmd = exec.CommandContext(ctx, "paplay", filePath)

	case "ffplay":
		cmd = exec.CommandContext(ctx, "ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", filePath)

	case "vlc":
		cmd = exec.CommandContext(ctx, "vlc", "--play-and-exit", "--intf", "dummy", filePath)

	case "mplayer":
		cmd = exec.CommandContext(ctx, "mplayer", "-noconsolecontrols", filePath)

	case "powershell":
		psScript := fmt.Sprintf(`
			Add-Type -AssemblyName presentationCore
			$media = New-Object system.windows.media.mediaplayer
			$media.open('%s')
			$media.Play()
			Start-Sleep -Seconds $media.NaturalDuration.TimeSpan.TotalSeconds
			$media.Stop()
			$media.Close()
		`, filePath)
		cmd = exec.CommandContext(ctx, "powershell", "-Command", psScript)

	default:
		return fmt.Errorf("audio-player: unsupported player: %s", p.playerCmd)
	}

	p.mu.Lock()
	p.currentCmd = cmd
	p.mu.Unlock()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.Canceled {
			return nil
		}
		return fmt.Errorf("audio-player: playback failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func (p *LocalAudioPlayer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentCmd != nil && p.currentCmd.Process != nil {
		p.currentCmd.Process.Kill()
	}
	p.isPlaying = false
	return nil
}

func (p *LocalAudioPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying
}

func (p *LocalAudioPlayer) SetVolume(vol float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if vol >= 0 && vol <= 2.0 {
		p.volume = vol
	}
}

func (p *LocalAudioPlayer) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

func (p *LocalAudioPlayer) SetPlayer(cmd string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playerCmd = cmd
}

func (p *LocalAudioPlayer) Player() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playerCmd
}

func (p *LocalAudioPlayer) AvailablePlayers() []string {
	players := []string{}

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("afplay"); err == nil {
			players = append(players, "afplay")
		}

	case "linux":
		for _, p := range []string{"aplay", "paplay", "ffplay", "vlc", "mplayer"} {
			if _, err := exec.LookPath(p); err == nil {
				players = append(players, p)
			}
		}

	case "windows":
		if _, err := exec.LookPath("powershell"); err == nil {
			players = append(players, "powershell")
		}
	}

	return players
}

type BufferAudioPlayer struct {
	mu        sync.Mutex
	isPlaying bool
	buffer    []byte
	format    AudioFormat
	playCh    chan struct{}
	stopCh    chan struct{}
}

func NewBufferAudioPlayer() *BufferAudioPlayer {
	return &BufferAudioPlayer{
		playCh: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
}

func (p *BufferAudioPlayer) Play(ctx context.Context, audio []byte, format AudioFormat) error {
	p.mu.Lock()
	if p.isPlaying {
		select {
		case <-p.stopCh:
		default:
		}
	}

	p.buffer = audio
	p.format = format
	p.isPlaying = true
	p.mu.Unlock()

	select {
	case p.playCh <- struct{}{}:
	default:
	}

	select {
	case <-ctx.Done():
		p.Stop()
		return ctx.Err()
	case <-p.stopCh:
		return nil
	}
}

func (p *BufferAudioPlayer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.isPlaying = false
	select {
	case p.stopCh <- struct{}{}:
	default:
	}

	return nil
}

func (p *BufferAudioPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying
}

func (p *BufferAudioPlayer) Buffer() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.buffer...)
}

func (p *BufferAudioPlayer) Format() AudioFormat {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.format
}

type MultiAudioPlayer struct {
	mu      sync.Mutex
	players []AudioPlayer
	current AudioPlayer
}

func NewMultiAudioPlayer(players ...AudioPlayer) *MultiAudioPlayer {
	return &MultiAudioPlayer{
		players: players,
	}
}

func (p *MultiAudioPlayer) Play(ctx context.Context, audio []byte, format AudioFormat) error {
	p.mu.Lock()

	if p.current == nil && len(p.players) > 0 {
		p.current = p.players[0]
	}

	player := p.current
	p.mu.Unlock()

	if player == nil {
		return fmt.Errorf("audio-player: no players available")
	}

	return player.Play(ctx, audio, format)
}

func (p *MultiAudioPlayer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.current != nil {
		return p.current.Stop()
	}
	return nil
}

func (p *MultiAudioPlayer) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.current != nil {
		return p.current.IsPlaying()
	}
	return false
}

func (p *MultiAudioPlayer) AddPlayer(player AudioPlayer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.players = append(p.players, player)
}

func (p *MultiAudioPlayer) SetPlayer(index int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.players) {
		return fmt.Errorf("audio-player: player index %d out of range", index)
	}

	p.current = p.players[index]
	return nil
}

func (p *MultiAudioPlayer) Players() []AudioPlayer {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]AudioPlayer(nil), p.players...)
}

type NopAudioPlayer struct{}

func (NopAudioPlayer) Play(ctx context.Context, audio []byte, format AudioFormat) error {
	return nil
}

func (NopAudioPlayer) Stop() error {
	return nil
}

func (NopAudioPlayer) IsPlaying() bool {
	return false
}
