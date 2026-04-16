package runtime

import (
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/orchestrator"
	runtimebootstrap "github.com/anyclaw/anyclaw/pkg/runtime/bootstrap"
)

func buildOrchestratorConfig(cfg *config.Config, workDir string, workingDir string) orchestrator.OrchestratorConfig {
	return runtimebootstrap.BuildOrchestratorConfig(cfg, workDir, workingDir)
}
