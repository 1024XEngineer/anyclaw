package speech

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type testAudioPlayer struct {
	playing bool
	audio   []byte
	format  AudioFormat
}

func (p *testAudioPlayer) Play(ctx context.Context, audio []byte, format AudioFormat) error {
	_ = ctx
	p.playing = true
	p.audio = append([]byte(nil), audio...)
	p.format = format
	return nil
}

func (p *testAudioPlayer) Stop() error {
	p.playing = false
	return nil
}

func (p *testAudioPlayer) IsPlaying() bool {
	return p.playing
}

func TestLocalAudioPlayerHelpers(t *testing.T) {
	tempDir := t.TempDir()
	defaultCfg := DefaultLocalAudioPlayerConfig()
	if defaultCfg.Volume != 1.0 {
		t.Fatalf("DefaultLocalAudioPlayerConfig() = %+v, want Volume=1.0", defaultCfg)
	}

	player := NewLocalAudioPlayer(LocalAudioPlayerConfig{
		TempDir:   tempDir,
		PlayerCmd: "custom-player",
		Volume:    5,
	})

	if player.Player() != "custom-player" {
		t.Fatalf("Player() = %q, want custom-player", player.Player())
	}
	if player.Volume() != 1.0 {
		t.Fatalf("Volume() = %v, want 1.0", player.Volume())
	}

	tmpPath, err := player.writeTempFile([]byte("audio"), FormatWAV)
	if err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile(temp): %v", err)
	}
	if string(data) != "audio" {
		t.Fatalf("unexpected temp file contents: %q", data)
	}

	player.SetVolume(1.5)
	player.SetVolume(9)
	if player.Volume() != 1.5 {
		t.Fatalf("Volume() after updates = %v, want 1.5", player.Volume())
	}

	player.SetPlayer("")
	if err := player.Play(context.Background(), []byte("audio"), FormatMP3); err == nil {
		t.Fatal("expected Play without player command to fail")
	}

	player.SetPlayer("unsupported")
	if err := player.playFile(context.Background(), tmpPath, FormatMP3); err == nil {
		t.Fatal("expected playFile with unsupported player to fail")
	}
	if player.IsPlaying() {
		t.Fatal("expected unsupported playback to reset playing state")
	}

	player.isPlaying = true
	if err := player.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if player.IsPlaying() {
		t.Fatal("expected Stop to clear playing state")
	}

	_ = detectPlayer()

	if players := player.AvailablePlayers(); players == nil {
		t.Fatal("AvailablePlayers() should not return nil")
	}
}

func TestBufferAndMultiAudioPlayers(t *testing.T) {
	buffer := NewBufferAudioPlayer()

	ctx, cancelRunning := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- buffer.Play(ctx, []byte("stream"), FormatWAV)
	}()

	select {
	case <-buffer.playCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for buffer player to start")
	}

	if !buffer.IsPlaying() {
		t.Fatal("expected buffer player to be playing")
	}

	cancelRunning()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Buffer Play returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for buffer player to return")
	}

	if buffer.IsPlaying() {
		t.Fatal("expected buffer player to stop")
	}
	if string(buffer.Buffer()) != "stream" || buffer.Format() != FormatWAV {
		t.Fatalf("unexpected buffer state: %q / %s", buffer.Buffer(), buffer.Format())
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := buffer.Play(cancelCtx, []byte("cancel"), FormatMP3); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if err := buffer.Stop(); err != nil {
		t.Fatalf("Buffer Stop: %v", err)
	}

	multi := NewMultiAudioPlayer()
	if err := multi.Play(context.Background(), []byte("x"), FormatPCM); err == nil {
		t.Fatal("expected empty multi player to fail")
	}

	player := &testAudioPlayer{}
	multi.AddPlayer(player)
	if err := multi.SetPlayer(1); err == nil {
		t.Fatal("expected invalid player index to fail")
	}
	if err := multi.SetPlayer(0); err != nil {
		t.Fatalf("SetPlayer(0): %v", err)
	}
	if err := multi.Play(context.Background(), []byte("abc"), FormatPCM); err != nil {
		t.Fatalf("Multi Play: %v", err)
	}
	if !multi.IsPlaying() {
		t.Fatal("expected multi player to report playing")
	}
	if err := multi.Stop(); err != nil {
		t.Fatalf("Multi Stop: %v", err)
	}
	if multi.IsPlaying() {
		t.Fatal("expected multi player to stop")
	}
	if got := multi.Players(); len(got) != 1 {
		t.Fatalf("Players() len = %d, want 1", len(got))
	}

	var nop NopAudioPlayer
	if err := nop.Play(context.Background(), []byte("abc"), FormatMP3); err != nil {
		t.Fatalf("Nop Play: %v", err)
	}
	if nop.IsPlaying() {
		t.Fatal("expected nop player to never report playing")
	}
	if err := nop.Stop(); err != nil {
		t.Fatalf("Nop Stop: %v", err)
	}
}