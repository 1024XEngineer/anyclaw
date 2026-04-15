package util

import "time"

// Now exposes time.Now through one helper to simplify later testing hooks.
func Now() time.Time {
	return time.Now()
}
