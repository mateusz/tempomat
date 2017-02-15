package bucket

import (
	"net/http"
	"strings"

	log "github.com/Sirupsen/logrus"
)

type Bucket interface {
	Register(r *http.Request, cost float64)
	Dump(l *log.Logger, lowCreditLogThreshold bool)
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
