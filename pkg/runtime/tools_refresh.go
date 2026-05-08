package runtime

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/markettools"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

// RefreshToolRegistry rebuilds tools that capture runtime config such as
// sandbox, approval, and network permissions. It intentionally leaves LLM,
// memory, and plugin lifecycle untouched.
func (a *MainRuntime) RefreshToolRegistry() error {
	if a == nil || a.Config == nil {
		return fmt.Errorf("runtime config is unavailable")
	}
	workingDir := a.WorkingDir
	if workingDir == "" {
		workingDir = a.Config.Agent.WorkingDir
	}
	if workingDir == "" {
		workingDir = a.Config.Agent.WorkDir
	}
	if workingDir == "" {
		workingDir = "workflows"
	}
	if profile, ok := a.Config.ResolveMainAgentProfile(); ok {
		if strings.TrimSpace(profile.WorkingDir) != "" {
			workingDir = profile.WorkingDir
		}
	}
	memoryWorkDir := memory.CodexMemoryWorkspaceDir(a.WorkDir, workingDir)

	refreshedSkills, _, err := reloadRuntimeSkills(a.Config)
	if err != nil {
		return fmt.Errorf("reload skills: %w", err)
	}
	a.Skills = refreshedSkills
	if a.Agent != nil {
		a.Agent.SetSkills(refreshedSkills)
	}

	registry := tools.NewRegistry()
	sandboxManager := tools.NewSandboxManager(a.Config.Sandbox, workingDir)
	permissionOptions := tools.PermissionOptions{
		SandboxMode:    a.Config.Permissions.SandboxMode,
		ApprovalPolicy: a.Config.Permissions.ApprovalPolicy,
		NetworkAccess:  a.Config.Permissions.NetworkAccess,
		DesktopAccess:  a.Config.Permissions.DesktopAccess,
	}
	permissionEngine := tools.NewPermissionEngine(workingDir, permissionOptions)
	policyEngine := tools.NewPolicyEngine(tools.PolicyOptions{
		WorkingDir:           workingDir,
		PermissionLevel:      a.Config.Agent.PermissionLevel,
		Permissions:          permissionOptions,
		ProtectedPaths:       a.Config.Security.ProtectedPaths,
		AllowedReadPaths:     a.Config.Security.AllowedReadPaths,
		AllowedWritePaths:    a.Config.Security.AllowedWritePaths,
		AllowedEgressDomains: a.Config.Security.AllowedEgressDomains,
	})
	var qmdClient tools.QMDClient
	if a.QMD != nil {
		qmdClient = &qmdAdapter{client: a.QMD}
	}
	var auditLogger tools.AuditLogger
	if a.Audit != nil {
		auditLogger = a.Audit
	}

	builtinOpts := tools.BuiltinOptions{
		WorkingDir:            workingDir,
		PermissionLevel:       a.Config.Agent.PermissionLevel,
		Permissions:           permissionOptions,
		ExecutionMode:         runtimeExecutionMode(a.Config.Sandbox.ExecutionMode, sandboxManager),
		DangerousPatterns:     a.Config.Security.DangerousCommandPatterns,
		ProtectedPaths:        a.Config.Security.ProtectedPaths,
		AllowedReadPaths:      a.Config.Security.AllowedReadPaths,
		AllowedWritePaths:     a.Config.Security.AllowedWritePaths,
		AllowedEgressDomains:  a.Config.Security.AllowedEgressDomains,
		Policy:                policyEngine,
		PermissionEngine:      permissionEngine,
		CommandTimeoutSeconds: a.Config.Security.CommandTimeoutSeconds,
		AuditLogger:           auditLogger,
		Sandbox:               sandboxManager,
		Computer: tools.ComputerOptions{
			Enabled:               a.Config.Computer.Enabled,
			Backend:               a.Config.Computer.Backend,
			CoordinateSpace:       a.Config.Computer.CoordinateSpace,
			MaxActionsPerTurn:     a.Config.Computer.MaxActionsPerTurn,
			ObserveAfterAction:    a.Config.Computer.ObserveAfterAction,
			IncludeWindowsDefault: a.Config.Computer.IncludeWindowsDefault,
			RedactTextInAudit:     a.Config.Computer.RedactTextInAudit,
			AllowedApps:           a.Config.Computer.AllowedApps,
			AllowedDomains:        a.Config.Computer.AllowedDomains,
		},
		MemoryBackend:  a.Memory,
		DailyMemoryDir: filepath.Join(memoryWorkDir, "daily"),
		QMDClient:      qmdClient,
	}
	tools.RegisterBuiltins(registry, builtinOpts)
	markettools.Register(registry, markettools.Options{
		Store:            marketplace.NewStore(marketplaceStoreRoot(a.WorkDir, workingDir)),
		Registry:         marketplaceRegistryClient(a.Config.Marketplace),
		AutoInstallSkill: a.Config.Marketplace.AutoInstallSkill,
		AuditLogger:      auditLogger,
		AfterInstall:     a.IntegrateMarketReceiptAndRefresh,
		AfterBind:        a.RefreshAfterMarketBinding,
	})
	if a.Skills != nil {
		a.Skills.RegisterTools(registry, skills.ExecutionOptions{AllowExec: a.Config.Plugins.AllowExec, ExecTimeoutSeconds: a.Config.Plugins.ExecTimeoutSeconds})
	}
	if a.Plugins != nil {
		a.Plugins.SetPolicyEngine(policyEngine)
		a.Plugins.RegisterToolPlugins(registry, a.Config.Plugins.Dir)
	}
	a.Tools = registry
	if a.Agent != nil {
		a.Agent.SetTools(registry)
	}

	if a.Orchestrator != nil {
		orchCfg := buildOrchestratorConfig(a.Config, a.WorkDir, workingDir)
		orchCfg.ToolOptions = &builtinOpts
		a.Orchestrator.SetAgentDefinitions(orchCfg.AgentDefinitions)
		a.Orchestrator.SetToolOptions(builtinOpts, registry)
	}
	return nil
}

func runtimeExecutionMode(configured string, sandbox *tools.SandboxManager) string {
	mode := strings.TrimSpace(strings.ToLower(configured))
	if mode == "" {
		mode = "sandbox"
	}
	if mode == "sandbox" && (sandbox == nil || !sandbox.Enabled()) {
		return "host-reviewed"
	}
	return mode
}
