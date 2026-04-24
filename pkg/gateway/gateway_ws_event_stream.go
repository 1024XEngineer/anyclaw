package gateway

import "github.com/1024XEngineer/anyclaw/pkg/state"

func (c *openClawWSConn) startEventStream() {
	if c.eventStream != nil || c.server.bus == nil {
		return
	}
	ch := c.server.bus.Subscribe(32)
	c.eventStream = ch
	go func() {
		for {
			select {
			case <-c.closed:
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				if !c.canSeeEvent(event) {
					continue
				}
				if err := c.writeFrame(openClawWSFrame{
					Type:  "event",
					Event: "events.updated",
					Data:  event,
				}); err != nil {
					c.shutdown()
					return
				}
			}
		}
	}()
}

func (c *openClawWSConn) stopEventStream() {
	if c.eventStream == nil || c.server.bus == nil {
		return
	}
	c.server.bus.Unsubscribe(c.eventStream)
	c.eventStream = nil
}

func (c *openClawWSConn) canSeeEvent(event *state.Event) bool {
	if event == nil || event.SessionID == "" || c.user == nil || c.user.Role == "admin" {
		return true
	}
	session, ok := c.server.sessions.Get(event.SessionID)
	if !ok {
		return true
	}
	orgID, projectID, workspaceID := state.SessionExecutionHierarchy(session)
	return HasHierarchyAccess(c.user, orgID, projectID, workspaceID)
}
