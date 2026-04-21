package gateway

import (
	"net/http"

	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
)

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	_ = gatewaysurface.Service{}.Write(w, gatewaysurface.WriteOutput{
		StatusCode: statusCode,
		Payload:    value,
	})
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	_ = gatewaysurface.Service{}.WriteError(w, statusCode, message)
}
