package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	log "github.com/Sirupsen/logrus"
)

type Slash32 struct {
	hash              map[string]entry
	hashMutex         sync.Mutex
	trustedProxiesMap map[string]bool
}

func NewSlash32(trustedProxiesMap map[string]bool) *Slash32 {
	return &Slash32{
		hash:              make(map[string]entry),
		hashMutex:         sync.Mutex{},
		trustedProxiesMap: trustedProxiesMap,
	}
}

type entry struct {
	remoteIp string
	credit   float64
}

func (b *Slash32) Register(r *http.Request, cost float64) {
	var err error
	ip := "0.0.0.0"
	ip, _, err = net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if _, ok := b.trustedProxiesMap[ip]; ok {
			headerIp := getIPAdressFromHeaders(r, b.trustedProxiesMap)
			if headerIp != "" {
				ip = headerIp
			}
		}
	}

	hash := md5.New()
	io.WriteString(hash, ip)
	key := fmt.Sprintf("%x", hash.Sum(nil))

	b.hashMutex.Lock()
	if c, ok := b.hash[key]; ok {
		c.credit -= cost
		b.hash[key] = c
	} else {
		b.hash[key] = entry{
			remoteIp: ip,
			credit:   -cost,
		}
	}
	b.hashMutex.Unlock()
}

func (b *Slash32) Dump(l *log.Logger) {
	for k, c := range b.hash {
		l.Info(fmt.Sprintf("%s,%s,%.3f", k, c.remoteIp, c.credit))
	}
	b.hashMutex.Lock()
	b.hash = make(map[string]entry)
	b.hashMutex.Unlock()
}
