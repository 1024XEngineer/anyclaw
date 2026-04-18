package runtime

import (
	"github.com/1024XEngineer/anyclaw/pkg/config"
	runtimebootstrap "github.com/1024XEngineer/anyclaw/pkg/runtime/bootstrap"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/orchestrator"
)

func buildOrchestratorConfig(cfg *config.Config, workDir string, workingDir string) orchestrator.OrchestratorConfig {
	return runtimebootstrap.BuildOrchestratorConfig(cfg, workDir, workingDir)
}
