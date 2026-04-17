package ingress

import "strings"

type ChannelRequest struct {
	Channel     string
	SessionID   string
	ReplyTarget string
	Message     string
	ThreadID    string
	IsGroup     bool
	GroupID     string
}

type Service struct {
	router *Router
}

func NewService(router *Router) *Service {
	return &Service{router: router}
}

func (s *Service) DecideChannel(req ChannelRequest) SessionRoute {
	if s == nil || s.router == nil {
		return SessionRoute{}
	}
	routeSource := strings.TrimSpace(req.SessionID)
	if replyTarget := strings.TrimSpace(req.ReplyTarget); replyTarget != "" {
		routeSource = replyTarget
	}
	return s.router.Decide(RouteRequest{
		Channel:  req.Channel,
		Source:   routeSource,
		Text:     req.Message,
		ThreadID: req.ThreadID,
		IsGroup:  req.IsGroup,
		GroupID:  req.GroupID,
	})
}
