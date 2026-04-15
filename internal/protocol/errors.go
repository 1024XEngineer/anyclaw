package protocol

// ErrorPayload is the shared machine-readable error model.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	ErrCodeUnauthorized  = "unauthorized"
	ErrCodeForbidden     = "forbidden"
	ErrCodeInvalidFrame  = "invalid_frame"
	ErrCodeUnknownMethod = "unknown_method"
	ErrCodeInternal      = "internal_error"
)
