package state

func SessionExecutionBindingValue(session *Session) SessionExecutionBinding {
	return sessionExecutionBindingValue(session)
}

func SessionExecutionAgent(session *Session) string {
	return sessionExecutionAgent(session)
}

func SessionExecutionWorkspace(session *Session) string {
	return sessionExecutionBindingValue(session).Workspace
}

func SessionExecutionHierarchy(session *Session) (string, string, string) {
	binding := sessionExecutionBindingValue(session)
	return binding.Org, binding.Project, binding.Workspace
}

func SessionExecutionTarget(session *Session) (string, string, string, string) {
	binding := sessionExecutionBindingValue(session)
	return binding.Agent, binding.Org, binding.Project, binding.Workspace
}

func CloneEvent(event *Event) *Event {
	return cloneEvent(event)
}

func CloneApproval(approval *Approval) *Approval {
	return cloneApproval(approval)
}

func CloneTaskRecoveryPoint(point *TaskRecoveryPoint) *TaskRecoveryPoint {
	return cloneTaskRecoveryPoint(point)
}

func NormalizeParticipants(primary string, participants []string) []string {
	return normalizeParticipants(primary, participants)
}

func ShortenTitle(input string) string {
	return shortenTitle(input)
}

func UniqueID(prefix string) string {
	return uniqueID(prefix)
}
