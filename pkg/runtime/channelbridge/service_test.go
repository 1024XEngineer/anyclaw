package channelbridge

import (
	"context"
	"fmt"
	"testing"

	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type recordedEvent struct {
	eventType string
	sessionID string
	payload   map[string]any
}

type eventRecorder struct {
	events []recordedEvent
}

func TestEnsureSessionCreatesSessionWithResolvedResources(t *testing.T) {
	service, sessions, recorder := newServiceTest(t)

	routed := routeingress.RouteOutput{
		Request: routeingress.RoutedRequest{
			Route: routeingress.RouteResolution{
				Session: routeingress.SessionResolution{
					NeedsCreate: true,
					SessionKey:  "conv-1",
					SessionMode: "group",
					QueueMode:   "serial",
					ReplyBack:   true,
				},
				Delivery: routeingress.DeliveryTarget{
					ReplyTo:  "reply-1",
					ThreadID: "thread-1",
					TransportMeta: map[string]string{
						"chat_id":          "chat-1",
						"guild_id":         "guild-1",
						"attachment_count": "3",
						"ignored":          "skip-me",
					},
				},
			},
		},
	}

	sessionID, err := service.EnsureSession("slack", "fallback-session", routed, map[string]string{
		"user_id":   "user-1",
		"user_name": "Alice",
	}, false)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	session, ok := sessions.Get(sessionID)
	if !ok {
		t.Fatalf("expected session %s to exist", sessionID)
	}
	if session.Agent != "main-agent" {
		t.Fatalf("expected fallback agent main-agent, got %q", session.Agent)
	}
	if session.Org != "org-1" || session.Project != "project-1" || session.Workspace != "workspace-1" {
		t.Fatalf("unexpected resource binding: org=%q project=%q workspace=%q", session.Org, session.Project, session.Workspace)
	}
	if session.SessionMode != "channel-dm" {
		t.Fatalf("expected group mode to normalize to channel-dm, got %q", session.SessionMode)
	}
	if session.QueueMode != "serial" {
		t.Fatalf("expected queue mode serial, got %q", session.QueueMode)
	}
	if !session.ReplyBack {
		t.Fatal("expected reply_back to be preserved")
	}
	if session.ReplyTarget != "reply-1" || session.ThreadID != "thread-1" {
		t.Fatalf("unexpected delivery target binding: reply=%q thread=%q", session.ReplyTarget, session.ThreadID)
	}
	if session.ConversationKey != "conv-1" {
		t.Fatalf("expected conversation key conv-1, got %q", session.ConversationKey)
	}
	if session.SourceChannel != "slack" {
		t.Fatalf("expected source channel slack, got %q", session.SourceChannel)
	}
	if session.SourceID != "fallback-session" {
		t.Fatalf("expected fallback source id, got %q", session.SourceID)
	}
	if session.TransportMeta["chat_id"] != "chat-1" || session.TransportMeta["guild_id"] != "guild-1" || session.TransportMeta["attachment_count"] != "3" {
		t.Fatalf("unexpected transport meta: %#v", session.TransportMeta)
	}
	if _, ok := session.TransportMeta["ignored"]; ok {
		t.Fatalf("unexpected transport meta key copied into session: %#v", session.TransportMeta)
	}

	if len(recorder.events) != 1 || recorder.events[0].eventType != "session.created" {
		t.Fatalf("expected one session.created event, got %#v", recorder.events)
	}
	if got := recorder.events[0].payload["title"]; got != "Slack session" {
		t.Fatalf("expected event title Slack session, got %#v", got)
	}
	if got := recorder.events[0].payload["source"]; got != "slack" {
		t.Fatalf("expected event source slack, got %#v", got)
	}
}

func TestEnsureSessionReusesExistingRoutedSession(t *testing.T) {
	service, sessions, recorder := newServiceTest(t)
	session, err := sessions.Create("Existing title", "main-agent", "org-1", "project-1", "workspace-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	routed := routeingress.RouteOutput{
		Request: routeingress.RoutedRequest{
			Route: routeingress.RouteResolution{
				Session: routeingress.SessionResolution{
					SessionID: session.ID,
					Created:   true,
				},
			},
		},
	}

	gotID, err := service.EnsureSession("discord", "ignored", routed, map[string]string{"user_id": "user-2"}, true)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if gotID != session.ID {
		t.Fatalf("expected existing session id %q, got %q", session.ID, gotID)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("expected one event, got %d", len(recorder.events))
	}
	if got := recorder.events[0].payload["title"]; got != "Existing title" {
		t.Fatalf("expected existing session title to be reused, got %#v", got)
	}
	if got := recorder.events[0].payload["streaming"]; got != true {
		t.Fatalf("expected streaming flag true, got %#v", got)
	}
}

func TestRunRoutedReturnsRunnerInitializationErrorAfterMaterializingSession(t *testing.T) {
	service, sessions, _ := newServiceTest(t)

	routed := routeingress.RouteOutput{
		Request: routeingress.RoutedRequest{
			Route: routeingress.RouteResolution{
				Session: routeingress.SessionResolution{
					NeedsCreate: true,
				},
			},
		},
	}

	response, session, err := service.RunRouted(context.Background(), "slack", "fallback", "hello", routed, map[string]string{"user_id": "u-1"})
	if err == nil || err.Error() != "session runner not initialized" {
		t.Fatalf("expected runner initialization error, got response=%q session=%#v err=%v", response, session, err)
	}
	if response != "" || session != nil {
		t.Fatalf("expected no response/session on runner init error, got response=%q session=%#v", response, session)
	}
	if len(sessions.List()) != 1 {
		t.Fatalf("expected session to be materialized before runner error, got %d sessions", len(sessions.List()))
	}
}

func TestNormalizedChannelHelpers(t *testing.T) {
	meta := map[string]string{
		"user_id": "user-1",
		"chat_id": "existing-chat",
	}
	target := routeingress.DeliveryTarget{
		ChannelID:      "channel-1",
		ConversationID: "conversation-1",
		ReplyTo:        "reply-1",
		ThreadID:       "thread-1",
		TransportMeta: map[string]string{
			"guild_id": "guild-1",
			"empty":    "   ",
		},
	}

	normalized := normalizedChannelRunMeta(meta, target)
	if normalized["channel"] != "channel-1" || normalized["channel_id"] != "channel-1" {
		t.Fatalf("expected channel ids to be normalized, got %#v", normalized)
	}
	if normalized["chat_id"] != "existing-chat" {
		t.Fatalf("expected existing chat_id to win, got %#v", normalized["chat_id"])
	}
	if normalized["conversation_id"] != "conversation-1" {
		t.Fatalf("expected conversation_id to be populated, got %#v", normalized["conversation_id"])
	}
	if normalized["reply_target"] != "reply-1" || normalized["thread_id"] != "thread-1" || normalized["guild_id"] != "guild-1" {
		t.Fatalf("unexpected normalized metadata: %#v", normalized)
	}

	if got := channelSourceID(map[string]string{"user_id": "user-2"}, "fallback"); got != "user-2" {
		t.Fatalf("expected user_id to win for channel source id, got %q", got)
	}
	if got := channelSourceID(map[string]string{"reply_target": "reply-2"}, "fallback"); got != "reply-2" {
		t.Fatalf("expected reply_target to win when user_id missing, got %q", got)
	}
	if got := channelSourceID(map[string]string{}, "fallback"); got != "fallback" {
		t.Fatalf("expected fallback source id, got %q", got)
	}

	transportMeta := channelSessionTransportMeta(map[string]string{
		"channel_id":       "channel-2",
		"attachment_count": "2",
		"skip":             "nope",
	})
	if len(transportMeta) != 2 || transportMeta["channel_id"] != "channel-2" || transportMeta["attachment_count"] != "2" {
		t.Fatalf("unexpected filtered transport meta: %#v", transportMeta)
	}

	if got := normalizeSingleAgentSessionMode("group-shared", "channel-dm"); got != "channel-dm" {
		t.Fatalf("expected group-shared to normalize to fallback, got %q", got)
	}
	if !isGroupSessionMode("channel-group") {
		t.Fatal("expected channel-group to be treated as group mode")
	}
	if got := firstNonEmpty("   ", " value ", "later"); got != "value" {
		t.Fatalf("expected first trimmed non-empty value, got %q", got)
	}
}

func newServiceTest(t *testing.T) (Service, *state.SessionManager, *eventRecorder) {
	t.Helper()

	store, err := state.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.UpsertOrg(&state.Org{ID: "org-1", Name: "Org"}); err != nil {
		t.Fatalf("UpsertOrg: %v", err)
	}
	if err := store.UpsertProject(&state.Project{ID: "project-1", OrgID: "org-1", Name: "Project"}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if err := store.UpsertWorkspace(&state.Workspace{ID: "workspace-1", ProjectID: "project-1", Name: "Workspace", Path: t.TempDir()}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	sessions := state.NewSessionManager(store, nil)
	recorder := &eventRecorder{}
	service := Service{
		Sessions:             sessions,
		ResolveMainAgentName: func() string { return "main-agent" },
		ResolveDefaultResources: func() (string, string, string) {
			return "org-1", "project-1", "workspace-1"
		},
		ValidateResourceSelection: func(orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error) {
			org, ok := store.GetOrg(orgID)
			if !ok {
				return nil, nil, nil, fmt.Errorf("org not found: %s", orgID)
			}
			project, ok := store.GetProject(projectID)
			if !ok {
				return nil, nil, nil, fmt.Errorf("project not found: %s", projectID)
			}
			workspace, ok := store.GetWorkspace(workspaceID)
			if !ok {
				return nil, nil, nil, fmt.Errorf("workspace not found: %s", workspaceID)
			}
			return org, project, workspace, nil
		},
		AppendEvent: func(eventType string, sessionID string, payload map[string]any) {
			cloned := map[string]any{}
			for k, v := range payload {
				cloned[k] = v
			}
			recorder.events = append(recorder.events, recordedEvent{eventType: eventType, sessionID: sessionID, payload: cloned})
		},
	}

	return service, sessions, recorder
}
