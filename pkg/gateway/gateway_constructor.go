package gateway

import (
	"strings"
	"time"

	openaicompat "github.com/1024XEngineer/anyclaw/pkg/api/openai"
	"github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	webhookext "github.com/1024XEngineer/anyclaw/pkg/extensions/adapters/webhook"
	appsecurity "github.com/1024XEngineer/anyclaw/pkg/gateway/auth/security"
	gatewaymiddleware "github.com/1024XEngineer/anyclaw/pkg/gateway/middleware"
	nodepkg "github.com/1024XEngineer/anyclaw/pkg/gateway/resources/nodes"
	"github.com/1024XEngineer/anyclaw/pkg/runtime"
	sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func New(mainRuntime *runtime.MainRuntime) *Server {
	store, err := state.NewStore(mainRuntime.WorkDir)
	if err != nil {
		panic(err)
	}
	server := &Server{
		mainRuntime:    mainRuntime,
		store:          store,
		sessions:       state.NewSessionManager(store, mainRuntime),
		bus:            state.NewEventBus(),
		runtimePool:    runtime.NewRuntimePool(mainRuntime.ConfigPath, store, mainRuntime.Config.Gateway.RuntimeMaxInstances, time.Duration(mainRuntime.Config.Gateway.RuntimeIdleSeconds)*time.Second),
		auth:           newAuthMiddleware(&mainRuntime.Config.Security),
		rateLimit:      newGatewayRateLimiter(mainRuntime),
		plugins:        mainRuntime.PluginRegistry(),
		telegram:       nil,
		jobQueue:       make(chan func(), 64),
		jobCancel:      map[string]bool{},
		jobMaxAttempts: mainRuntime.Config.Gateway.JobMaxAttempts,
		webhooks:       newWebhookHandler(),
		nodes:          newNodeManager(),
		devicePairing:  newDevicePairing(mainRuntime),
	}
	server.approvals = state.NewApprovalManager(store)
	server.sessionRunner = sessionrunner.NewManager(store, server.sessions, server.runtimePool, server.approvals, sessionEventRecorder{server: server})
	server.tasks = taskrunner.NewManager(store, server.sessions, server.runtimePool, taskrunner.MainRuntimeInfo{
		Name:       mainRuntime.Config.Agent.Name,
		WorkingDir: mainRuntime.WorkingDir,
		ConfigPath: mainRuntime.ConfigPath,
	}, mainRuntime, server.approvals)

	if sm, err := agentstore.NewStoreManager(mainRuntime.WorkDir, mainRuntime.ConfigPath); err == nil {
		server.storeModule = sm
	}

	server.openAICompat = newOpenAICompatHandler(server, mainRuntime)
	return server
}

func newGatewayRateLimiter(mainRuntime *runtime.MainRuntime) *gatewaymiddleware.RateLimiter {
	return gatewaymiddleware.NewRateLimiter(&mainRuntime.Config.Security)
}

func newWebhookHandler() *webhookext.Handler {
	return webhookext.NewHandler()
}

func newNodeManager() *nodepkg.DeviceManager {
	return nodepkg.NewDeviceManager()
}

func newDevicePairing(mainRuntime *runtime.MainRuntime) *appsecurity.DevicePairing {
	pairing := appsecurity.NewDevicePairing(mainRuntime.Config.Security.PairingTTLHours)
	if mainRuntime.Config.Security.PairingEnabled {
		pairing.SetEnabled(true)
	}
	return pairing
}

func newOpenAICompatHandler(server *Server, mainRuntime *runtime.MainRuntime) *openaicompat.Handler {
	return openaicompat.NewHandler(
		func(requestedModel string) (*runtime.App, string, error) {
			if err := server.ensureDefaultWorkspace(); err != nil {
				return nil, "", err
			}
			agentName := strings.TrimSpace(requestedModel)
			if agentName == "" {
				agentName = mainRuntime.Config.ResolveMainAgentName()
			}
			orgID, projectID, workspaceID := defaultResourceIDs(mainRuntime.WorkingDir)
			targetApp, err := server.runtimePool.GetOrCreate(agentName, orgID, projectID, workspaceID)
			if err != nil {
				return nil, "", err
			}
			return targetApp, agentName, nil
		},
		func() []string {
			names := make([]string, 0, len(mainRuntime.Config.Agent.Profiles)+2)
			for _, profile := range mainRuntime.Config.Agent.Profiles {
				if name := strings.TrimSpace(profile.Name); name != "" {
					names = append(names, name)
				}
			}
			if mainName := strings.TrimSpace(mainRuntime.Config.ResolveMainAgentName()); mainName != "" {
				names = append(names, mainName)
			}
			if model := strings.TrimSpace(mainRuntime.Config.LLM.Model); model != "" {
				names = append(names, model)
			}
			return names
		},
	)
}
