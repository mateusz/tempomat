package bucket

import (
	"net/http"
	"strings"
	"sync"

	"github.com/mateusz/tempomat/lib/config"
	"time"
)

type Bucket struct {
	delayThreshold time.Duration
	cpuCount	float64
	rate           float64
	hashMaxLen     int
	sync.RWMutex
}

func (b *Bucket) SetConfig(c config.Config) {
	b.Lock()
	defer b.Unlock()

	b.delayThreshold = time.Duration(c.DelayThresholdSec*1000) * time.Millisecond
	b.hashMaxLen = c.HashMaxLen
}

func (b *Bucket) DelayThreshold() time.Duration {
	b.RLock()
	defer b.RUnlock()

	return b.delayThreshold
}

func getIPAdressFromHeaders(r *http.Request, m map[string]bool) string {
	for _, h := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		header := r.Header.Get(h)
		if header == "" {
			continue
		}

		addresses := strings.Split(header, ",")
		ip := ""
		for i := len(addresses) - 1; i >= 0; i-- {
			ip = strings.TrimSpace(addresses[i])
			if _, ok := m[ip]; !ok {
				break
			}
		}
		return ip
	}
	return ""
}
