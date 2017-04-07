package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/mateusz/tempomat/lib/config"
)

type UserAgent struct {
	Bucket
	hash map[string]EntryUserAgent
}

func NewUserAgent(rate float64, hashMaxLen int) *UserAgent {
	b := &UserAgent{
		Bucket: Bucket{
			rate:       rate,
			hashMaxLen: hashMaxLen,
		},
		hash: make(map[string]EntryUserAgent),
	}
	go b.ticker()
	return b
}

func (b *UserAgent) SetConfig(c config.Config) {
	b.Lock()
	b.rate = c.UserAgentShare
	b.Unlock()

	b.Bucket.SetConfig(c)
}

func (b *UserAgent) String() string {
	return "UserAgent"
}

func (b *UserAgent) Entries() Entries {
	b.RLock()
	defer b.RUnlock()

	l := make(Entries, len(b.hash))
	i := 0
	for _, v := range b.hash {
		l[i] = v
		i++
	}
	return l
}

func (b *UserAgent) Register(r *http.Request, cost float64) {
	ua := r.UserAgent()

	b.Lock()
	entry := EntryUserAgent{
		userAgent: ua,
		credit:    b.rate*10.0 - cost,
	}
	key := entry.Hash()

	// subtract the credits since this it's already in ther
	if c, ok := b.hash[key]; ok {
		c.credit -= cost
		b.hash[key] = c
		// new entry, give it full credits - 1
	} else {
		b.hash[key] = entry
	}

	log.Info(fmt.Sprintf("UserAgent: %s, %f billed to '%s', total is %f", r.URL, cost, ua, b.hash[key].credit))
	b.Unlock()
}

// Not concurrency safe.
func (b *UserAgent) truncate(truncatedSize int) {
	newHash := make(map[string]EntryUserAgent)

	entries := b.Entries()
	sort.Sort(CreditSortEntries(entries))
	for i := 0; i < truncatedSize; i++ {
		newHash[entries[i].Hash()] = entries[i].(EntryUserAgent)
	}
	b.hash = newHash
}

func (b *UserAgent) ticker() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		b.Lock()
		for k, c := range b.hash {
			// Remove entries that are at their max credits
			if c.credit+b.rate > b.rate*10.0 {
				delete(b.hash, k)
			} else {
				c.credit += b.rate
				b.hash[k] = c
			}
		}
		// Truncate some entries to not blow out the memory
		if len(b.hash) > b.hashMaxLen {
			b.truncate(b.hashMaxLen)
		}
		b.Unlock()
	}
}

type EntryUserAgent struct {
	userAgent string
	credit    float64
}

func (e EntryUserAgent) Hash() string {
	hasher := md5.New()
	io.WriteString(hasher, e.userAgent)
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func (e EntryUserAgent) Credit() float64 {
	return e.credit
}

func (e EntryUserAgent) String() string {
	return fmt.Sprintf("%s,%.3f", e.userAgent, e.credit)
}

func (e EntryUserAgent) Title() string {
	return e.userAgent
}
