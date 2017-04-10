package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/mateusz/tempomat/lib/config"
)

type Slash32 struct {
	Bucket
	hash              map[string]EntrySlash32
	trustedProxiesMap map[string]bool
	netmask           int
}

func NewSlash32(rate float64, trustedProxiesMap map[string]bool, netmask int, hashMaxLen int) *Slash32 {
	b := &Slash32{
		Bucket: Bucket{
			rate:       rate,
			hashMaxLen: hashMaxLen,
		},
		hash:              make(map[string]EntrySlash32),
		trustedProxiesMap: trustedProxiesMap,
		netmask:           netmask,
	}
	go b.ticker()
	return b
}

func (b *Slash32) SetConfig(c config.Config) {
	b.Lock()
	switch b.netmask {
	case 32:
		b.rate = c.Slash32Share
	case 24:
		b.rate = c.Slash24Share
	case 16:
		b.rate = c.Slash16Share
	}
	b.Unlock()

	b.Bucket.SetConfig(c)
}

func (b *Slash32) String() string {
	b.RLock()
	defer b.RUnlock()
	return fmt.Sprintf("Slash%d", b.netmask)
}

func (b *Slash32) Netmask() int {
	b.RLock()
	defer b.RUnlock()
	return b.netmask
}

func (b *Slash32) Entries() Entries {
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

func (b *Slash32) Register(r *http.Request, cost float64) {
	var err error
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if _, ok := b.trustedProxiesMap[ip]; ok {
			headerIp := getIPAdressFromHeaders(r, b.trustedProxiesMap)
			if headerIp != "" {
				ip = headerIp
			}
		}
	}

	ipnet := "0.0.0.0/0"
	_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip, b.netmask))
	// @todo shouldn't this error be handled properly? is it ipv6 compat?
	if err == nil {
		ipnet = network.String()
	}

	b.Lock()
	entry := EntrySlash32{
		netmask: ipnet,
		credit:  b.rate*10.0 - cost,
	}
	key := entry.Hash()

	if c, ok := b.hash[key]; ok {
		c.credit -= cost
		b.hash[key] = c
	} else {
		b.hash[key] = entry
	}
	log.Info(fmt.Sprintf("Slash%d: %s, %f billed to '%s' (%s), total is %f", b.netmask, r.URL, cost, ipnet, ip, b.hash[key].credit))
	b.Unlock()
}

// Not concurrency safe.
func (b *Slash32) truncate(truncatedSize int) {
	newHash := make(map[string]EntrySlash32)

	entries := b.Entries()
	sort.Sort(CreditSortEntries(entries))
	for i := 0; i < truncatedSize; i++ {
		newHash[entries[i].Hash()] = entries[i].(EntrySlash32)
	}
	b.hash = newHash
}

func (b *Slash32) ticker() {
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

type EntrySlash32 struct {
	netmask string
	credit  float64
}

func (e EntrySlash32) Hash() string {
	hasher := md5.New()
	io.WriteString(hasher, e.netmask)
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func (e EntrySlash32) Credit() float64 {
	return e.credit
}

func (e EntrySlash32) String() string {
	return fmt.Sprintf("%s,%.3f", e.netmask, e.credit)
}

func (e EntrySlash32) Title() string {
	return e.netmask
}
