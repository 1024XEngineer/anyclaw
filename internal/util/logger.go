package util

import "log"

// NewLogger returns the standard logger for now.
func NewLogger() *log.Logger {
	return log.Default()
}
