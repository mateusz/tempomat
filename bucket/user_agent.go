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

func NewUserAgent(c config.Config) *UserAgent {
	b := &UserAgent{
		hash: make(map[string]EntryUserAgent),
	}
	b.SetConfig(c)
	go b.ticker()
	return b
}

func (b *UserAgent) SetConfig(c config.Config) {
	b.Lock()
	b.rate = c.UserAgentCPUs
	b.truncate(0)
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

func (b *UserAgent) entries() Entries {
	l := make(Entries, len(b.hash))
	i := 0
	for _, v := range b.hash {
		l[i] = v
		i++
	}
	return l
}

func (b *UserAgent) ReserveN(r *http.Request, start time.Time, qty float64) (delay time.Duration, ok bool) {
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
		entry.limiter = rate.NewLimiter(rate.Limit(b.rate * 1000), 120 * 1000)
		b.hash[key] = entry
	}

	rsv := entry.limiter.ReserveN(start, int(qty * 1000))
	if rsv.OK() && rsv.Delay()!=rate.InfDuration {
		ok = true
		delay = rsv.Delay()
	} else {
		ok = false
		delay = 120 * time.Second
	}

	var delayRemaining time.Duration
	elapsed := time.Now().Sub(start)
	if elapsed<=delay {
		delayRemaining = delay-elapsed
	}

	sincePrev := time.Now().Sub(entry.lastUsed)
	if sincePrev>0 && sincePrev<time.Minute {
		entry.avgSincePrev -= entry.avgSincePrev/10
		entry.avgSincePrev += sincePrev/10
	}

	entry.lastUsed = time.Now()
	entry.avgWait -= entry.avgWait/10
	entry.avgWait += delayRemaining /10

	cpuSecsPerSec := qty/float64(entry.avgSincePrev.Seconds())
	if cpuSecsPerSec<100.0 {
		entry.avgCpuSecs -= entry.avgCpuSecs / 10
		entry.avgCpuSecs += cpuSecsPerSec / 10
	}
	b.hash[key] = entry

	return
}

// Not concurrency safe.
func (b *UserAgent) truncate(truncatedSize int) {
	entries := b.entries()

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

	// Note: this will overwrite recently added entries
	b.hash = newHash
}

func (b *UserAgent) ticker() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		b.Lock()
		b.truncate(b.hashMaxLen)
		b.Unlock()
	}
}

type EntryUserAgent struct {
	userAgent    string
	lastUsed     time.Time
	avgWait      time.Duration
	avgSincePrev time.Duration
	avgCpuSecs   float64
	limiter      *rate.Limiter
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

func (e EntryUserAgent) AvgSincePrev() time.Duration {
	return e.avgSincePrev
}

func (e EntryUserAgent) AvgCpuSecs() float64 {
	return e.avgCpuSecs
}

func (e EntryUserAgent) String() string {
	return fmt.Sprintf("%s, used %d ago", e.userAgent, time.Now().Sub(e.lastUsed).Seconds())
}

func (e EntryUserAgent) Title() string {
	return e.userAgent
}
