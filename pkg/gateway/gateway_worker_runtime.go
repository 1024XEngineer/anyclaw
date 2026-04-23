package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func RunWithWorkers(ctx context.Context, mainRuntime *runtime.MainRuntime) error {
	workerCount := mainRuntime.Config.Gateway.WorkerCount
	if workerCount <= 0 {
		workerCount = 4
	}
	if workerCount > 64 {
		workerCount = 64
	}

	if os.Getenv("ANYCLAW_WORKER_MODE") == "1" {
		return runWorker(ctx, mainRuntime)
	}

	return runMaster(ctx, mainRuntime, workerCount)
}

func runMaster(ctx context.Context, mainRuntime *runtime.MainRuntime, workerCount int) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	basePort := mainRuntime.Config.Gateway.Port
	workerPIDs := make([]int, workerCount)
	workerPorts := make([]int, workerCount)

	for i := 0; i < workerCount; i++ {
		workerPort := basePort + i
		workerPorts[i] = workerPort
		cmd := exec.Command(execPath, "gateway", "run",
			"--config", mainRuntime.ConfigPath,
			"--host", mainRuntime.Config.Gateway.Host,
			"--port", strconv.Itoa(workerPort),
			"--workers", "1")
		cmd.Env = append(os.Environ(),
			"ANYCLAW_WORKER_MODE=1",
			fmt.Sprintf("ANYCLAW_WORKER_ID=%d", i),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			for _, pid := range workerPIDs[:i] {
				killProcess(pid)
			}
			return fmt.Errorf("start worker %d: %w", i, err)
		}
		workerPIDs[i] = cmd.Process.Pid
	}

	printWorkerStatus(workerPIDs, workerPorts, basePort)

	<-ctx.Done()

	for _, pid := range workerPIDs {
		killProcess(pid)
	}

	return nil
}

func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func runWorker(ctx context.Context, mainRuntime *runtime.MainRuntime) error {
	workerID := os.Getenv("ANYCLAW_WORKER_ID")
	addr := runtime.GatewayAddress(mainRuntime.Config)

	server := New(mainRuntime)
	mux := http.NewServeMux()

	server.initChannels()
	if err := server.ensureDefaultWorkspace(); err != nil {
		return err
	}
	server.startWorkers(ctx)
	server.registerWorkerRoutes(mux)

	server.startedAt = time.Now().UTC()
	server.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go server.runChannels(ctx)
	go func() {
		if err := server.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("worker %s server failed: %w", workerID, err)
	}
}

func printWorkerStatus(pids []int, ports []int, basePort int) {
	if len(pids) == 0 {
		return
	}
	fmt.Printf("Gateway workers started:\n")
	for i, pid := range pids {
		addr := fmt.Sprintf("127.0.0.1:%d", ports[i])
		fmt.Printf("  Worker %d: PID=%d, addr=%s\n", i, pid, addr)
	}
	fmt.Printf("Main Gateway: 127.0.0.1:%d (load balancer)\n", basePort)
}
