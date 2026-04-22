package state

import (
	"fmt"
	"sync/atomic"
	"time"
)

var stateIDCounter uint64

func uniqueID(prefix string) string {
	seq := atomic.AddUint64(&stateIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
