package server

import (
	"fmt"
	"sync/atomic"
	"time"
)

var idCounter uint64

func newID(prefix string) string {
	count := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), count)
}
