package gateway

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func StartDetached(mainRuntime *runtime.MainRuntime) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := Probe(ctx, runtime.GatewayURL(mainRuntime.Config)); err == nil {
		return fmt.Errorf("gateway already running at %s", runtime.GatewayURL(mainRuntime.Config))
	}
	logPath := mainRuntime.Config.Daemon.LogFile
	if logPath == "" {
		logPath = filepath.Join(mainRuntime.WorkDir, "gateway.log")
	}
	pidPath := mainRuntime.Config.Daemon.PIDFile
	if pidPath == "" {
		pidPath = filepath.Join(mainRuntime.WorkDir, "gateway.pid")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], "gateway", "run", "--config", mainRuntime.ConfigPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	startCtx, startCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer startCancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		probeCtx, probeCancel := context.WithTimeout(startCtx, time.Second)
		_, err := Probe(probeCtx, runtime.GatewayURL(mainRuntime.Config))
		probeCancel()
		if err == nil {
			pidData := []byte(strconv.Itoa(cmd.Process.Pid))
			return os.WriteFile(pidPath, pidData, 0o644)
		}
		select {
		case <-startCtx.Done():
			return fmt.Errorf("gateway daemon failed to start within 5s; see %s", logPath)
		case <-ticker.C:
		}
	}
}

func StopDetached(mainRuntime *runtime.MainRuntime) error {
	pidPath := mainRuntime.Config.Daemon.PIDFile
	if pidPath == "" {
		pidPath = filepath.Join(mainRuntime.WorkDir, "gateway.pid")
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, probeErr := Probe(ctx, runtime.GatewayURL(mainRuntime.Config)); probeErr != nil {
			_ = os.Remove(pidPath)
			return nil
		}
		return err
	}
	_ = os.Remove(pidPath)
	return nil
}
