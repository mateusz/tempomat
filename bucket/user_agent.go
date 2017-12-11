package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"golang.org/x/time/rate"
	"github.com/mateusz/tempomat/lib/config"
)

type UserAgent struct {
	Bucket
	hash map[string]EntryUserAgent
}

func NewUserAgent(c config.Config, cpuCount float64) *UserAgent {
	b := &UserAgent{
		Bucket: Bucket{
			cpuCount:       cpuCount,
		},
		hash: make(map[string]EntryUserAgent),
	}
	b.SetConfig(c)
	go b.ticker()
	return b
}

func (b *UserAgent) SetConfig(c config.Config) {
	b.Lock()
	b.rate = c.UserAgentShare * b.cpuCount
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

func (b *UserAgent) ReserveN(r *http.Request, start time.Time, qty float64) *rate.Reservation {
	ua := r.UserAgent()

	b.Lock()
	defer b.Unlock()
	entry := EntryUserAgent{
		userAgent: ua,
	}
	key := entry.Hash()

	if _, ok := b.hash[key]; ok {
		entry = b.hash[key];
	} else {
		entry.limiter = rate.NewLimiter(rate.Limit(b.rate * 1000), 10 * 1000)
		b.hash[key] = entry
	}


	rsv := entry.limiter.ReserveN(start, int(qty * 1000))

	entry.lastUsed = time.Now()
	entry.avgWait -= entry.avgWait/10
	entry.avgWait += rsv.Delay()/10
	b.hash[key] = entry

	return rsv
}

// Not concurrency safe.
func (b *UserAgent) truncate(truncatedSize int) {
	entries := b.Entries()

	sort.Sort(LastUsedSortEntries(entries))
	purged := make(Entries, 0, len(entries))
	for i := 0; i < len(entries); i++ {
		if time.Now().Sub(entries[i].LastUsed())<60*time.Second {
			purged = append(purged, entries[i])
		}
	}

	sort.Sort(AvgWaitSortEntries(purged))
	newHash := make(map[string]EntryUserAgent)
	for i := 0; i < truncatedSize && i<len(purged); i++ {
		newHash[purged[i].Hash()] = purged[i].(EntryUserAgent)
	}

	// TODO this will overwrite recently added entries
	b.Lock()
	defer b.Unlock()
	b.hash = newHash
}

func (b *UserAgent) ticker() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		// Truncate some entries to not blow out the memory
		if len(b.hash) > b.hashMaxLen {
			b.truncate(b.hashMaxLen)
		}
	}
}

type EntryUserAgent struct {
	userAgent string
	lastUsed time.Time
	avgWait time.Duration
	limiter *rate.Limiter
}

func (e EntryUserAgent) Hash() string {
	hasher := md5.New()
	io.WriteString(hasher, e.userAgent)
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func (e EntryUserAgent) LastUsed() time.Time {
	return e.lastUsed
}

func (e EntryUserAgent) AvgWait() time.Duration {
	return e.avgWait
}

func (e EntryUserAgent) String() string {
	return fmt.Sprintf("%s, used %d ago", e.userAgent, time.Now().Sub(e.lastUsed).Seconds())
}

func (e EntryUserAgent) Title() string {
	return e.userAgent
}
