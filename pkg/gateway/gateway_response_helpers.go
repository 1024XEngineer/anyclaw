package gateway

import (
	"net/http"

	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
)

func writeError(w http.ResponseWriter, statusCode int, message string) {
	_ = gatewaysurface.Service{}.WriteError(w, statusCode, message)
}
