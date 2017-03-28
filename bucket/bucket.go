package bucket

import (
	"net/http"
	"strings"
	"sync"
)

type Bucket struct {
	rate       float64
	hashMaxLen int
	mutex      sync.RWMutex
}

func (b *Bucket) SetRate(rate float64) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.rate = rate
}

func (b *Bucket) SetHashMaxLen(hashMaxLen int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.hashMaxLen = hashMaxLen
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
