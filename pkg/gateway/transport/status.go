package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
	runtimepkg "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type Status struct {
	OK         bool   `json:"ok"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Address    string `json:"address"`
	StartedAt  string `json:"started_at,omitempty"`
	WorkingDir string `json:"working_dir"`
	WorkDir    string `json:"work_dir"`
	Sessions   int    `json:"sessions"`
	Events     int    `json:"events"`
	Skills     int    `json:"skills"`
	Tools      int    `json:"tools"`
	Secured    bool   `json:"secured"`
	Users      int    `json:"users"`
}

type GatewayStatus struct {
	Status    Status         `json:"status"`
	Health    HealthStatus   `json:"health"`
	Presence  PresenceStatus `json:"presence"`
	Typing    TypingStatus   `json:"typing"`
	Approvals ApprovalStatus `json:"approvals"`
	Sessions  SessionStatus  `json:"sessions"`
	Channels  ChannelStatus  `json:"channels"`
	Security  SecurityStatus `json:"security"`
	Runtime   RuntimeStatus  `json:"runtime"`
	UpdatedAt string         `json:"updated_at"`
}

type HealthStatus struct {
	OK            bool   `json:"ok"`
	Uptime        string `json:"uptime"`
	ChannelsUp    int    `json:"channels_up"`
	ChannelsTotal int    `json:"channels_total"`
	LLMConnected  bool   `json:"llm_connected"`
	LastError     string `json:"last_error,omitempty"`
}

type PresenceStatus struct {
	ActiveUsers int            `json:"active_users"`
	ByChannel   map[string]int `json:"by_channel"`
}

type TypingStatus struct {
	ActiveSessions int            `json:"active_sessions"`
	ByChannel      map[string]int `json:"by_channel"`
}

type ApprovalStatus struct {
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Denied   int `json:"denied"`
	Total    int `json:"total"`
}

type SessionStatus struct {
	Total     int            `json:"total"`
	Active    int            `json:"active"`
	Idle      int            `json:"idle"`
	Queued    int            `json:"queued"`
	ByChannel map[string]int `json:"by_channel"`
}

type ChannelStatus struct {
	Total  int                      `json:"total"`
	ByName map[string]AdapterStatus `json:"by_name"`
}

type AdapterStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type SecurityStatus struct {
	DMPolicy         string `json:"dm_policy"`
	GroupPolicy      string `json:"group_policy"`
	MentionGate      bool   `json:"mention_gate"`
	PairingEnabled   bool   `json:"pairing_enabled"`
	RiskAcknowledged bool   `json:"risk_acknowledged"`
	AllowFromCount   int    `json:"allow_from_count"`
}

type RuntimeStatus struct {
	Pooled int `json:"pooled"`
	Active int `json:"active"`
	Idle   int `json:"idle"`
	Max    int `json:"max"`
}

type ChannelStatusProvider interface {
	Statuses() []inputlayer.Status
}

type RuntimePoolStatusProvider interface {
	Status() runtimepkg.PoolStatus
}

type StatusDeps struct {
	MainRuntime       *runtimepkg.MainRuntime
	StartedAt         time.Time
	Store             *state.Store
	Channels          ChannelStatusProvider
	RuntimePool       RuntimePoolStatusProvider
	EnabledSkillCount func() int
}

const TypingSessionStaleAfter = 20 * time.Second

func StatusSnapshot(deps StatusDeps) Status {
	status := Status{
		OK:      true,
		Status:  "running",
		Version: runtimepkg.Version,
	}
	if !deps.StartedAt.IsZero() {
		status.StartedAt = deps.StartedAt.Format(time.RFC3339)
	}
	if deps.MainRuntime != nil {
		status.WorkingDir = deps.MainRuntime.WorkingDir
		status.WorkDir = deps.MainRuntime.WorkDir
		status.Tools = len(deps.MainRuntime.ListTools())
		if deps.MainRuntime.Config != nil {
			status.Provider = deps.MainRuntime.Config.LLM.Provider
			status.Model = deps.MainRuntime.Config.LLM.Model
			status.Address = runtimepkg.GatewayAddress(deps.MainRuntime.Config)
			status.Secured = deps.MainRuntime.Config.Security.APIToken != ""
			status.Users = len(deps.MainRuntime.Config.Security.Users)
		}
	}
	if deps.Store != nil {
		status.Sessions = len(deps.Store.ListSessions())
		status.Events = len(deps.Store.ListEvents(0))
	}
	if deps.EnabledSkillCount != nil {
		status.Skills = deps.EnabledSkillCount()
	}
	return status
}

func GatewaySnapshot(deps StatusDeps) GatewayStatus {
	sessions := []*state.Session{}
	if deps.Store != nil {
		sessions = deps.Store.ListSessions()
	}
	activeUsers := make(map[string]bool)
	now := time.Now().UTC()
	typingSessions := 0
	queuedSessions := 0
	channelSessions := make(map[string]int)
	for _, sess := range sessions {
		if sess.UserID != "" {
			activeUsers[sess.UserID] = true
		}
		if sess.Typing && TypingSessionActive(sess, now, TypingSessionStaleAfter) {
			typingSessions++
		}
		if sess.Presence == "queued" {
			queuedSessions++
		}
		if sess.SourceChannel != "" {
			channelSessions[sess.SourceChannel]++
		}
	}

	approvals := []*state.Approval{}
	tasks := []*state.Task{}
	if deps.Store != nil {
		approvals = deps.Store.ListApprovals("")
		tasks = deps.Store.ListTasks()
	}
	approvalStatus := summarizeApprovals(approvals, sessions, tasks)

	channelStatuses := []inputlayer.Status{}
	if deps.Channels != nil {
		channelStatuses = deps.Channels.Statuses()
	}
	channelsUp := 0
	channelByName := make(map[string]AdapterStatus)
	for _, current := range channelStatuses {
		if current.Running && current.Healthy {
			channelsUp++
		}
		channelByName[current.Name] = AdapterStatus{
			Name:    current.Name,
			Enabled: current.Enabled,
			Running: current.Running,
			Healthy: current.Healthy,
			Error:   current.LastError,
		}
	}

	securityStatus := SecurityStatus{}
	if deps.MainRuntime != nil && deps.MainRuntime.Config != nil {
		secCfg := deps.MainRuntime.Config.Channels.Security
		securityStatus = SecurityStatus{
			DMPolicy:         secCfg.DMPolicy,
			GroupPolicy:      secCfg.GroupPolicy,
			MentionGate:      secCfg.MentionGate,
			PairingEnabled:   secCfg.PairingEnabled,
			RiskAcknowledged: deps.MainRuntime.Config.Security.RiskAcknowledged,
			AllowFromCount:   len(secCfg.AllowFrom),
		}
	}

	runtimeStatus := RuntimeStatus{}
	if deps.RuntimePool != nil {
		poolStatus := deps.RuntimePool.Status()
		runtimeStatus = RuntimeStatus{
			Pooled: poolStatus.Pooled,
			Active: poolStatus.Active,
			Idle:   poolStatus.Idle,
			Max:    poolStatus.Max,
		}
	}

	llmConnected := false
	uptime := ""
	if deps.MainRuntime != nil {
		llmConnected = deps.MainRuntime.HasLLM()
	}
	if !deps.StartedAt.IsZero() {
		uptime = time.Since(deps.StartedAt).Round(time.Second).String()
	}

	return GatewayStatus{
		Status: StatusSnapshot(deps),
		Health: HealthStatus{
			OK:            true,
			Uptime:        uptime,
			ChannelsUp:    channelsUp,
			ChannelsTotal: len(channelStatuses),
			LLMConnected:  llmConnected,
		},
		Presence: PresenceStatus{
			ActiveUsers: len(activeUsers),
			ByChannel:   nil,
		},
		Typing: TypingStatus{
			ActiveSessions: typingSessions,
			ByChannel:      nil,
		},
		Approvals: ApprovalStatus{
			Pending:  approvalStatus.Pending,
			Approved: approvalStatus.Approved,
			Denied:   approvalStatus.Denied,
			Total:    approvalStatus.Total,
		},
		Sessions: SessionStatus{
			Total:     len(sessions),
			Active:    len(sessions) - queuedSessions,
			Idle:      0,
			Queued:    queuedSessions,
			ByChannel: channelSessions,
		},
		Channels: ChannelStatus{
			Total:  len(channelStatuses),
			ByName: channelByName,
		},
		Security:  securityStatus,
		Runtime:   runtimeStatus,
		UpdatedAt: now.Format(time.RFC3339),
	}
}

func summarizeApprovals(approvals []*state.Approval, sessions []*state.Session, tasks []*state.Task) ApprovalStatus {
	sessionIDs := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session == nil || strings.TrimSpace(session.ID) == "" {
			continue
		}
		sessionIDs[strings.TrimSpace(session.ID)] = struct{}{}
	}
	taskIDs := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if task == nil || strings.TrimSpace(task.ID) == "" {
			continue
		}
		taskIDs[strings.TrimSpace(task.ID)] = struct{}{}
	}

	summary := ApprovalStatus{}
	for _, approval := range approvals {
		if !approvalCountsTowardsStatus(approval, sessionIDs, taskIDs) {
			continue
		}
		summary.Total++
		switch strings.TrimSpace(approval.Status) {
		case "pending":
			summary.Pending++
		case "approved":
			summary.Approved++
		case "denied", "rejected":
			summary.Denied++
		}
	}
	return summary
}

func approvalCountsTowardsStatus(approval *state.Approval, sessionIDs map[string]struct{}, taskIDs map[string]struct{}) bool {
	if approval == nil {
		return false
	}
	taskID := strings.TrimSpace(approval.TaskID)
	if taskID != "" {
		_, ok := taskIDs[taskID]
		return ok
	}
	sessionID := strings.TrimSpace(approval.SessionID)
	if sessionID != "" {
		_, ok := sessionIDs[sessionID]
		return ok
	}
	return true
}

func TypingSessionActive(session *state.Session, now time.Time, maxAge time.Duration) bool {
	if session == nil || !session.Typing {
		return false
	}
	if maxAge <= 0 {
		return true
	}
	last := session.LastActiveAt
	if last.IsZero() {
		last = session.UpdatedAt
	}
	if last.IsZero() {
		return true
	}
	return now.Sub(last) <= maxAge
}

func Probe(ctx context.Context, baseURL string) (*Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}
