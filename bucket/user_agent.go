package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type UserAgent struct {
	Bucket
	hash map[string]entryUserAgent
}

func NewUserAgent(rate float64, hashMaxLen int) *UserAgent {
	b := &UserAgent{
		Bucket: Bucket{
			rate:       rate,
			mutex:      sync.RWMutex{},
			hashMaxLen: hashMaxLen,
		},
		hash: make(map[string]entryUserAgent),
	}
	go b.ticker()
	return b
}

type entryUserAgent struct {
	UA     string
	Credit float64
}

func (b *UserAgent) SetHashMaxLen(hashMaxLen int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.hashMaxLen = hashMaxLen
}

func (b *UserAgent) Register(r *http.Request, cost float64) {
	ua := r.UserAgent()

	hash := md5.New()
	io.WriteString(hash, ua)
	key := fmt.Sprintf("%x", hash.Sum(nil))

	b.mutex.Lock()
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
	b.mutex.Unlock()
}

func (b *UserAgent) Dump(l *log.Logger, lowCreditLogThreshold float64) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	for k, c := range b.hash {
		if c.Credit <= (b.rate * 10.0 * lowCreditLogThreshold) {
			l.Info(fmt.Sprintf("UserAgent,%s,'%s',%.3f", k, c.UA, c.Credit))
		}
	}
}

func (b *UserAgent) DumpList() DumpList {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.dumpListNoLock()
}

func (b *UserAgent) dumpListNoLock() DumpList {
	l := make(DumpList, len(b.hash))
	i := 0
	for _, v := range b.hash {
		e := DumpEntry{Title: v.UA, Credit: v.Credit}
		l[i] = e
		i++
	}
	return l
}

func (b *UserAgent) truncate(truncatedSize int) {
	newHash := make(map[string]entryUserAgent)

	dumpList := b.dumpListNoLock()
	sort.Sort(CreditSortDumpList(dumpList))
	for i := 0; i < truncatedSize; i++ {
		newHash[dumpList[i].Hash] = entryUserAgent{
			UA:     dumpList[i].Title,
			Credit: dumpList[i].Credit,
		}
	}
	b.hash = newHash
}

func (b *UserAgent) ticker() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		b.mutex.Lock()
		if len(b.hash) > b.hashMaxLen {
			b.truncate(b.hashMaxLen)
		}
		for k, c := range b.hash {
			if c.Credit+b.rate > b.rate*10.0 {
				// Purge entries that are at their max credit.
				delete(b.hash, k)
			} else {
				c.Credit += b.rate
				b.hash[k] = c
			}
		}
		b.mutex.Unlock()
	}
}
