package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/rifflock/lfshook"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/load"
)

type config struct {
	debug             bool
	backend           string
	listenPort        int
	logFile           string
	statsFile         string
	trustedProxies    string
	trustedProxiesMap map[string]bool
}

var conf *config

type Client struct {
	RemoteIp string
	Credit   float64
}

var clientHash map[string]Client
var clientHashMutex sync.Mutex

var statsLog *log.Logger

func utilisation() (float64, error) {
	load, err := load.Avg()
	if err != nil {
		return -1, err
	}
	count, err := cpu.Counts(true)
	if err != nil {
		return -1, err
	}

	return load.Load1 / float64(count), nil
}

func init() {
	log.SetOutput(os.Stderr)
	log.SetLevel(log.WarnLevel)

	const (
		debugHelp          = "Enable debug output"
		backendHelp        = "Backend URI"
		listenPortHelp     = "Local HTTP listen port"
		logFileHelp        = "Log file"
		statsFileHelp      = "Stats file"
		trustedProxiesHelp = "Trusted proxy ips"
	)
	conf = &config{
		trustedProxiesMap: make(map[string]bool),
	}
	flag.BoolVar(&conf.debug, "debug", false, debugHelp)
	flag.StringVar(&conf.backend, "backend", "http://localhost:80", backendHelp)
	flag.IntVar(&conf.listenPort, "listen-port", 8888, listenPortHelp)
	flag.StringVar(&conf.logFile, "log-file", "", logFileHelp)
	flag.StringVar(&conf.statsFile, "stats-file", "", statsFileHelp)
	flag.StringVar(&conf.trustedProxies, "trusted-proxies", "", trustedProxiesHelp)
	flag.Parse()

	proxies := strings.Split(conf.trustedProxies, ",")
	for _, proxy := range proxies {
		conf.trustedProxiesMap[proxy] = true
	}

	if conf.logFile != "" {
		log.AddHook(lfshook.NewHook(lfshook.PathMap{
			log.PanicLevel: conf.logFile,
			log.FatalLevel: conf.logFile,
			log.ErrorLevel: conf.logFile,
			log.WarnLevel:  conf.logFile,
			log.InfoLevel:  conf.logFile,
			log.DebugLevel: conf.logFile,
		}))
	}

	statsLog = log.New()
	statsLog.Out = ioutil.Discard
	statsLog.Level = log.InfoLevel
	statsLog.Formatter = &StatsFormatter{}
	if conf.statsFile != "" {
		var err error
		statsLog.Out, err = os.OpenFile(conf.statsFile, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
	}

	if conf.debug {
		tw := tabwriter.NewWriter(os.Stdout, 24, 4, 1, ' ', tabwriter.AlignRight)
		fmt.Fprintf(tw, "Value\t   Option\f")
		fmt.Fprintf(tw, "%t\t - %s\f", conf.debug, debugHelp)
		fmt.Fprintf(tw, "%s\t - %s\f", conf.backend, backendHelp)
		fmt.Fprintf(tw, "%d\t - %s\f", conf.listenPort, listenPortHelp)
		fmt.Fprintf(tw, "%s\t - %s\f", conf.logFile, logFileHelp)
		fmt.Fprintf(tw, "%s\t - %s\f", conf.statsFile, statsFileHelp)
		fmt.Fprintf(tw, "%s\t - %s\f", conf.trustedProxies, trustedProxiesHelp)

		log.SetLevel(log.DebugLevel)
		statsLog.Level = log.DebugLevel
	}

	clientHash = make(map[string]Client)
	clientHashMutex = sync.Mutex{}
}

func getIPAdressFromHeaders(r *http.Request) string {
	for _, h := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		header := r.Header.Get(h)
		if header == "" {
			continue
		}

		addresses := strings.Split(header, ",")
		ip := ""
		for i := len(addresses) - 1; i >= 0; i-- {
			ip = strings.TrimSpace(addresses[i])
			if _, ok := conf.trustedProxiesMap[ip]; !ok {
				break
			}
		}
		return ip
	}
	return ""
}

func middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		ip := "0.0.0.0"
		ip, _, err = net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			if _, ok := conf.trustedProxiesMap[ip]; ok {
				headerIp := getIPAdressFromHeaders(r)
				if headerIp != "" {
					ip = headerIp
				}
			}
		}

		hash := md5.New()
		io.WriteString(hash, ip)
		key := fmt.Sprintf("%x", hash.Sum(nil))

		start := time.Now()
		h.ServeHTTP(w, r)
		reqTime := time.Since(start)

		u, err := utilisation()
		if err != nil {
			log.Warn(fmt.Sprintf("%s, assuming utilisation=1.0", err))
			u = 1.0
		}

		// Cost is expressed in the amount of compute seconds consumed.
		// It's scaled down by load average if the CPU is 100% saturated to take CPU contention into account.
		cost := float64(reqTime) / 1e9
		if u > 1.0 {
			cost = cost / u
		}
		log.Info(fmt.Sprintf("%s billed %f for %s", ip, cost, r.URL))

		clientHashMutex.Lock()
		if c, ok := clientHash[key]; ok {
			c.Credit -= cost
			clientHash[key] = c
		} else {
			clientHash[key] = Client{
				RemoteIp: ip,
				Credit:   -cost,
			}
		}
		clientHashMutex.Unlock()
	})
}

func statsLogger() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		for k, c := range clientHash {
			statsLog.Info(fmt.Sprintf("%s,%s,%.3f", k, c.RemoteIp, c.Credit))
		}
		clientHashMutex.Lock()
		clientHash = make(map[string]Client)
		clientHashMutex.Unlock()
	}
}

func listen() {
	url, err := url.Parse(conf.backend)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	handler := middleware(proxy)
	http.ListenAndServe(fmt.Sprintf(":%d", conf.listenPort), handler)
}

func main() {
	go statsLogger()
	listen()
}
