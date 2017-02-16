package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/mateusz/tempomat/api"
)

type UserAgent struct {
	Bucket
	hash      map[string]entryUserAgent
	hashMutex sync.Mutex
}

func NewUserAgent(rate float64) *UserAgent {
	b := &UserAgent{
		Bucket: Bucket{
			rate: rate,
		},
		hash:      make(map[string]entryUserAgent),
		hashMutex: sync.Mutex{},
	}
	go b.ticker()
	return b
}

type entryUserAgent struct {
	userAgent string
	credit    float64
}

func (b *UserAgent) Register(r *http.Request, cost float64) {
	ua := r.UserAgent()

	hash := md5.New()
	io.WriteString(hash, ua)
	key := fmt.Sprintf("%x", hash.Sum(nil))

	b.hashMutex.Lock()
	if c, ok := b.hash[key]; ok {
		c.credit -= cost
		b.hash[key] = c
	} else {
		b.hash[key] = entryUserAgent{
			userAgent: ua,
			credit:    b.rate*10.0 - cost,
		}
	}
	b.hashMutex.Unlock()

	log.Info(fmt.Sprintf("UserAgent: %s, %f billed to '%s', total is %f", r.URL, cost, ua, b.hash[key].credit))
}

func (b *UserAgent) Dump(l *log.Logger, lowCreditLogThreshold float64) {
	for k, c := range b.hash {
		if c.credit <= (b.rate * 10.0 * lowCreditLogThreshold) {
			l.Info(fmt.Sprintf("UserAgent,%s,'%s',%.3f", k, c.userAgent, c.credit))
		}
	}
}

func (b *UserAgent) DumpAll(args *api.EmptyArgs, reply *string) error {
	*reply = "bbb"
	return nil
}

func (b *UserAgent) ticker() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		for k, c := range b.hash {
			if c.credit+b.rate > b.rate*10.0 {
				c.credit = b.rate * 10.0
			} else {
				c.credit += b.rate
			}
			b.hash[k] = c
		}
	}
}
