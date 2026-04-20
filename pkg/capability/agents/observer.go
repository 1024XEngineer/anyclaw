package agent

type ToolActivity struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	Result   string         `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type Observer interface {
	OnToolActivity(activity ToolActivity)
}

func (a *Agent) SetObserver(observer Observer) {
	a.observerMu.Lock()
	a.observer = observer
	a.observerMu.Unlock()
}

func (a *Agent) GetLastToolActivities() []ToolActivity {
	a.observerMu.RLock()
	defer a.observerMu.RUnlock()
	return append([]ToolActivity(nil), a.lastToolActivities...)
}

func (a *Agent) resetToolActivities() {
	a.observerMu.Lock()
	a.lastToolActivities = nil
	a.observerMu.Unlock()
}

func (a *Agent) recordToolActivity(activity ToolActivity) {
	a.observerMu.Lock()
	a.lastToolActivities = append(a.lastToolActivities, activity)
	if len(a.lastToolActivities) > 50 {
		a.lastToolActivities = a.lastToolActivities[len(a.lastToolActivities)-50:]
	}
	observer := a.observer
	a.observerMu.Unlock()
	if observer != nil {
		observer.OnToolActivity(activity)
	}
}
