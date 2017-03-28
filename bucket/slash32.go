package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
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
			mutex:      sync.RWMutex{},
			hashMaxLen: hashMaxLen,
		},
		hash:              make(map[string]EntrySlash32),
		trustedProxiesMap: trustedProxiesMap,
		netmask:           netmask,
	}
	go b.ticker()
	return b
}

type EntrySlash32 struct {
	Netmask string
	Credit  float64
}

func (b *Slash32) SetHashMaxLen(hashMaxLen int) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.hashMaxLen = hashMaxLen
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

	ipnet := "0.0.0.0/0"
	var network *net.IPNet
	_, network, err = net.ParseCIDR(fmt.Sprintf("%s/%d", ip, b.netmask))
	if err == nil {
		ipnet = network.String()
	}

	hash := md5.New()
	io.WriteString(hash, ipnet)
	key := fmt.Sprintf("%x", hash.Sum(nil))

	b.mutex.Lock()
	if c, ok := b.hash[key]; ok {
		c.Credit -= cost
		b.hash[key] = c
	} else {
		b.hash[key] = EntrySlash32{
			Netmask: ipnet,
			Credit:  b.rate*10.0 - cost,
		}
	}

	log.Info(fmt.Sprintf("Slash%d: %s, %f billed to '%s' (%s), total is %f", b.netmask, r.URL, cost, ipnet, ip, b.hash[key].Credit))
	b.mutex.Unlock()
}

func (b *Slash32) Dump(l *log.Logger, lowCreditLogThreshold float64) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	for k, c := range b.hash {
		if c.Credit <= (b.rate * 10.0 * lowCreditLogThreshold) {
			l.Info(fmt.Sprintf("Slash%d,%s,%s,%.3f", b.netmask, k, c.Netmask, c.Credit))
		}
	}
}

func (b *Slash32) DumpList() DumpList {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.dumpListNoLock()
}

func (b *Slash32) dumpListNoLock() DumpList {
	l := make(DumpList, len(b.hash))
	i := 0
	for k, v := range b.hash {
		e := DumpEntry{Hash: k, Title: v.Netmask, Credit: v.Credit}
		l[i] = e
		i++
	}
	return l
}

func (b *Slash32) truncate(truncatedSize int) {
	newHash := make(map[string]EntrySlash32)

	dumpList := b.dumpListNoLock()
	sort.Sort(CreditSortDumpList(dumpList))
	for i := 0; i < truncatedSize; i++ {
		newHash[dumpList[i].Hash] = EntrySlash32{
			Netmask: dumpList[i].Title,
			Credit:  dumpList[i].Credit,
		}
	}
	b.hash = newHash
}

func (b *Slash32) ticker() {
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
