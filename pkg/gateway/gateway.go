package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	openaicompat "github.com/1024XEngineer/anyclaw/pkg/api/openai"
	agentstore "github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	webhookext "github.com/1024XEngineer/anyclaw/pkg/extensions/adapters/webhook"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/mcp"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	appsecurity "github.com/1024XEngineer/anyclaw/pkg/gateway/auth/security"
	"github.com/1024XEngineer/anyclaw/pkg/gateway/intake/chat"
	gatewaymiddleware "github.com/1024XEngineer/anyclaw/pkg/gateway/middleware"
	"github.com/1024XEngineer/anyclaw/pkg/gateway/resources/discovery"
	nodepkg "github.com/1024XEngineer/anyclaw/pkg/gateway/resources/nodes"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
	inputchannels "github.com/1024XEngineer/anyclaw/pkg/input/channels"
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
	"github.com/1024XEngineer/anyclaw/pkg/runtime"
	sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"
	taskrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/taskrunner"
	"github.com/1024XEngineer/anyclaw/pkg/speech"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

const defaultGatewayAddress = "127.0.0.1:18789"

type Server struct {
	mainRuntime    *runtime.MainRuntime
	httpServer     *http.Server
	startedAt      time.Time
	store          *state.Store
	sessions       *state.SessionManager
	bus            *state.EventBus
	channels       *inputlayer.Manager
	telegram       *inputchannels.TelegramAdapter
	slack          *inputchannels.SlackAdapter
	discord        *inputchannels.DiscordAdapter
	whatsapp       *inputchannels.WhatsAppAdapter
	signal         *inputchannels.SignalAdapter
	ingress        *routeingress.Service
	runtimePool    *runtime.RuntimePool
	sessionRunner  *sessionrunner.Manager
	tasks          *taskrunner.Manager
	chatModule     chat.ChatManager
	storeModule    agentstore.StoreManager
	approvals      *state.ApprovalManager
	auth           *authMiddleware
	rateLimit      *gatewaymiddleware.RateLimiter
	plugins        *plugin.Registry
	ingressPlugins []plugin.IngressRunner
	jobQueue       chan func()
	jobCancel      map[string]bool
	jobMaxAttempts int
	webhooks       *webhookext.Handler
	nodes          *nodepkg.DeviceManager
	openAICompat   *openaicompat.Handler
	sttPipeline    *speech.STTPipeline
	sttIntegration *speech.STTIntegration
	sttManager     *speech.STTManager
	ttsPipeline    *speech.TTSPipeline
	ttsIntegration *speech.Integration
	ttsManager     *speech.Manager
	mcpRegistry    *mcp.Registry
	mcpServer      *mcp.Server
	marketStore    *plugin.Store
	discoverySvc   *discovery.Service
	mentionGate    *inputlayer.MentionGate
	groupSecurity  *inputlayer.GroupSecurity
	channelCmds    *inputlayer.ChannelCommands
	channelPairing *inputlayer.ChannelPairing
	channelPolicy  *inputlayer.ChannelPolicy
	presenceMgr    *inputlayer.PresenceManager
	contactDir     *inputlayer.ContactDirectory
	devicePairing  *appsecurity.DevicePairing
}

func (s *Server) Run(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("gateway server is nil")
	}

	mux := http.NewServeMux()
	if err := s.ensureDefaultWorkspace(); err != nil {
		return err
	}
	s.startWorkers(ctx)
	s.registerGatewayRoutes(mux)

	s.startedAt = time.Now().UTC()
	s.httpServer = &http.Server{
		Addr:              s.address(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("gateway server failed: %w", err)
	}
}

func (s *Server) address() string {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Config == nil {
		return defaultGatewayAddress
	}
	addr := runtime.GatewayAddress(s.mainRuntime.Config)
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return defaultGatewayAddress
	}
	return addr
}
