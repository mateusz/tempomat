package bucket

import (
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type Slash32 struct {
	Bucket
	hash              map[string]EntrySlash32
	hashMutex         sync.Mutex
	trustedProxiesMap map[string]bool
	netmask           int
}

func NewSlash32(rate float64, trustedProxiesMap map[string]bool, netmask int) *Slash32 {
	b := &Slash32{
		Bucket: Bucket{
			rate: rate,
		},
		hash:              make(map[string]EntrySlash32),
		hashMutex:         sync.Mutex{},
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

	b.hashMutex.Lock()
	if c, ok := b.hash[key]; ok {
		c.Credit -= cost
		b.hash[key] = c
	} else {
		b.hash[key] = EntrySlash32{
			Netmask: ipnet,
			Credit:  b.rate*10.0 - cost,
		}
	}
	b.hashMutex.Unlock()

	log.Info(fmt.Sprintf("Slash32(%d): %s, %f billed to '%s' (%s), total is %f", b.netmask, r.URL, cost, ipnet, ip, b.hash[key].Credit))
}

func (b *Slash32) Dump(l *log.Logger, lowCreditLogThreshold float64) {
	fmt.Printf("%#v\n", len(b.hash))
	for k, c := range b.hash {
		if c.Credit <= (b.rate * 10.0 * lowCreditLogThreshold) {
			l.Info(fmt.Sprintf("Slash32(%d),%s,%s,%.3f", b.netmask, k, c.Netmask, c.Credit))
		}
	}
}

func (b *Slash32) DumpAll() (map[string]EntrySlash32, error) {
	h := make(map[string]EntrySlash32)
	for k, v := range b.hash {
		h[k] = v
	}
	return h, nil
}

func (b *Slash32) ticker() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		for k, c := range b.hash {
			if c.Credit+b.rate > b.rate*10.0 {
				c.Credit = b.rate * 10.0
			} else {
				c.Credit += b.rate
			}
			b.hash[k] = c
		}
	}
}
