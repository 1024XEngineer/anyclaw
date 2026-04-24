package gateway

import "net/http"

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	s.controlPlanePresenceAPI().Handle(w, r)
}
