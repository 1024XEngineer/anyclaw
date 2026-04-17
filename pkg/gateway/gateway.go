package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	openaicompat "github.com/anyclaw/anyclaw/pkg/api/openai"
	"github.com/anyclaw/anyclaw/pkg/capability/catalogs"
	webhookext "github.com/anyclaw/anyclaw/pkg/extensions/adapters/webhook"
	"github.com/anyclaw/anyclaw/pkg/extensions/mcp"
	"github.com/anyclaw/anyclaw/pkg/extensions/plugin"
	appsecurity "github.com/anyclaw/anyclaw/pkg/gateway/auth/security"
	"github.com/anyclaw/anyclaw/pkg/gateway/intake/chat"
	gatewaymiddleware "github.com/anyclaw/anyclaw/pkg/gateway/middleware"
	"github.com/anyclaw/anyclaw/pkg/gateway/resources/discovery"
	nodepkg "github.com/anyclaw/anyclaw/pkg/gateway/resources/nodes"
	inputlayer "github.com/anyclaw/anyclaw/pkg/input"
	inputchannels "github.com/anyclaw/anyclaw/pkg/input/channels"
	routeingress "github.com/anyclaw/anyclaw/pkg/route/ingress"
	"github.com/anyclaw/anyclaw/pkg/runtime"
	sessionrunner "github.com/anyclaw/anyclaw/pkg/runtime/sessionrunner"
	taskrunner "github.com/anyclaw/anyclaw/pkg/runtime/taskrunner"
	"github.com/anyclaw/anyclaw/pkg/speech"
	"github.com/anyclaw/anyclaw/pkg/state"
)

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
	addr := runtime.GatewayAddress(s.mainRuntime.Config)
	mux := http.NewServeMux()
	s.initChannels()
	s.initMCP(ctx)
	s.initMarketStore()
	s.initDiscovery(ctx)
	if err := s.ensureDefaultWorkspace(); err != nil {
		return err
	}
	s.startWorkers(ctx)
	s.registerGatewayRoutes(mux)

	s.startedAt = time.Now().UTC()
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go s.runChannels(ctx)
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
