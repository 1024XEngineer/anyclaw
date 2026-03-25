package gateway

import (
	"fmt"
	"sync/atomic"
	"time"
)

var gatewayIDCounter uint64

func uniqueID(prefix string) string {
	seq := atomic.AddUint64(&gatewayIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
