package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type UserAgent struct {
	Bucket
	hash      map[string]entryUserAgent
	hashMutex sync.RWMutex
}

func NewUserAgent(rate float64) *UserAgent {
	b := &UserAgent{
		Bucket: Bucket{
			rate: rate,
		},
		hash:      make(map[string]entryUserAgent),
		hashMutex: sync.RWMutex{},
	}
	go b.ticker()
	return b
}

type entryUserAgent struct {
	UA     string
	Credit float64
}

func (b *UserAgent) Register(r *http.Request, cost float64) {
	ua := r.UserAgent()

	hash := md5.New()
	io.WriteString(hash, ua)
	key := fmt.Sprintf("%x", hash.Sum(nil))

	b.hashMutex.Lock()
	if c, ok := b.hash[key]; ok {
		c.Credit -= cost
		b.hash[key] = c
	} else {
		b.hash[key] = entryUserAgent{
			UA:     ua,
			Credit: b.rate*10.0 - cost,
		}
	}

	log.Info(fmt.Sprintf("UserAgent: %s, %f billed to '%s', total is %f", r.URL, cost, ua, b.hash[key].Credit))
	b.hashMutex.Unlock()
}

func (b *UserAgent) Dump(l *log.Logger, lowCreditLogThreshold float64) {
	b.hashMutex.RLock()
	for k, c := range b.hash {
		if c.Credit <= (b.rate * 10.0 * lowCreditLogThreshold) {
			l.Info(fmt.Sprintf("UserAgent,%s,'%s',%.3f", k, c.UA, c.Credit))
		}
	}
	b.hashMutex.RUnlock()
}

func (b *UserAgent) DumpList() (DumpList, error) {
	l := make(DumpList, len(b.hash))
	i := 0
	b.hashMutex.RLock()
	for _, v := range b.hash {
		e := DumpEntry{Title: v.UA, Credit: v.Credit}
		l[i] = e
		i++
	}
	b.hashMutex.RUnlock()
	return l, nil
}

func (b *UserAgent) ticker() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		b.hashMutex.Lock()
		for k, c := range b.hash {
			if c.Credit+b.rate > b.rate*10.0 {
				c.Credit = b.rate * 10.0
			} else {
				c.Credit += b.rate
			}
			b.hash[k] = c
		}
		b.hashMutex.Unlock()
	}
}
