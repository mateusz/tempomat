package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/rpc"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/mateusz/tempomat/api"
	"github.com/mateusz/tempomat/bucket"
	"github.com/rifflock/lfshook"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/load"

	_ "net/http/pprof"
)

type config struct {
	Debug              bool    `json:"debug"`
	LowCreditThreshold float64 `json:"lowCreditThreshold"`
	Backend            string  `json:"backend"`
	ListenPort         int     `json:"listenPort"`
	LogFile            string  `json:"logFile"`
	StatsFile          string  `json:"statsFile"`
	Graphite           string  `json:"graphite"`
	GraphitePrefix     string  `json:"graphitePrefix"`
	TrustedProxies     string  `json:"trustedProxies"`
	Slash32Share       float64 `json:"slash32Share"`
	Slash24Share       float64 `json:"slash24Share"`
	Slash16Share       float64 `json:"slash16Share"`
	UserAgentShare     float64 `json:"userAgentShare"`
	HashMaxLen         int     `json:"hashMaxLen"`
	graphiteURL        *url.URL
	trustedProxiesMap  map[string]bool
}

func newConfig() config {
	return config{
		Debug:              false,
		LowCreditThreshold: 0.1,
		Backend:            "http://localhost:80",
		ListenPort:         8888,
		LogFile:            "",
		StatsFile:          "",
		Graphite:           "",
		GraphitePrefix:     "",
		TrustedProxies:     "",
		Slash32Share:       0.1,
		Slash24Share:       0.25,
		Slash16Share:       0.5,
		UserAgentShare:     0.1,
		HashMaxLen:         1000,
		graphiteURL:        nil,
		trustedProxiesMap:  make(map[string]bool),
	}
}

var conf config

var Slash32 *bucket.Slash32
var Slash24 *bucket.Slash32
var Slash16 *bucket.Slash32
var UserAgent *bucket.UserAgent
var statsLog *log.Logger
var cpuCount float64

func utilisation() (float64, error) {
	load, err := load.Avg()
	if err != nil {
		return -1, err
	}

	return load.Load1 / cpuCount, nil
}

func init() {
	log.SetOutput(os.Stderr)
	log.SetLevel(log.WarnLevel)

	conf = newConfig()

	jsonStr, err := ioutil.ReadFile("/etc/tempomat.json")
	if err != nil {
		log.Error(err)
		log.Fatal("Refusing to start on unreadable config file.")
	}
	if err = json.Unmarshal(jsonStr, &conf); err != nil {
		log.Error(err)
		log.Fatal("Refusing to start on unparseable config file.")
	}

	proxies := strings.Split(conf.TrustedProxies, ",")
	for _, proxy := range proxies {
		conf.trustedProxiesMap[proxy] = true
	}

	if conf.LogFile != "" {
		log.AddHook(lfshook.NewHook(lfshook.PathMap{
			log.PanicLevel: conf.LogFile,
			log.FatalLevel: conf.LogFile,
			log.ErrorLevel: conf.LogFile,
			log.WarnLevel:  conf.LogFile,
			log.InfoLevel:  conf.LogFile,
			log.DebugLevel: conf.LogFile,
		}))
	}

	statsLog = log.New()
	statsLog.Out = ioutil.Discard
	statsLog.Level = log.InfoLevel
	statsLog.Formatter = &StatsFormatter{}
	if conf.StatsFile != "" {
		var err error
		statsLog.Out, err = os.OpenFile(conf.StatsFile, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			log.Fatal(err)
		}
	}

	if conf.Debug {
		log.SetLevel(log.DebugLevel)
		statsLog.Level = log.DebugLevel

		printConf()
	}

	cpuCountInt, err := cpu.Counts(true)
	if err != nil {
		log.Fatal(err)
	}
	cpuCount = float64(cpuCountInt)

	if conf.Graphite != "" {
		if conf.GraphitePrefix == "" {
			log.Fatal("Configuration failure: 'graphitePrefix' is required if 'graphite' is specified")
		}
		var err error
		conf.graphiteURL, err = url.Parse(conf.Graphite)
		if err != nil {
			log.Fatal(err)
		}
	}

	Slash32 = bucket.NewSlash32(cpuCount*conf.Slash32Share, conf.trustedProxiesMap, 32, conf.HashMaxLen)
	Slash24 = bucket.NewSlash32(cpuCount*conf.Slash24Share, conf.trustedProxiesMap, 24, conf.HashMaxLen)
	Slash16 = bucket.NewSlash32(cpuCount*conf.Slash16Share, conf.trustedProxiesMap, 16, conf.HashMaxLen)
	UserAgent = bucket.NewUserAgent(cpuCount*conf.UserAgentShare, conf.HashMaxLen)
}

func sendMetric(metric string, value string) {
	dialer, err := net.Dial("tcp", conf.graphiteURL.Host)
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to connect to graphite server: %s", err))
		return
	}

	dialer.Write([]byte(fmt.Sprintf("%s.%s %s %d\n", conf.GraphitePrefix, metric, value, time.Now().Unix())))
	dialer.Close()
}

func printConf() {
	const (
		debugHelp              = "Debug mode"
		lowCreditThresholdHelp = "Low credit threshold"
		backendHelp            = "Backend URI"
		listenPortHelp         = "Local HTTP listen port"
		logFileHelp            = "Log file"
		statsFileHelp          = "Stats file"
		graphiteHelp           = "Graphite server, e.g. 'tcp://localhost:2003'"
		graphitePrefixHelp     = "Graphite prefix, exclude final dot, e.g. 'chaos.schmall.prod'"
		trustedProxiesHelp     = "Trusted proxy ips"
		slash32ShareHelp       = "Slash32 max CPU share"
		slash24ShareHelp       = "Slash24 max CPU share"
		slash16ShareHelp       = "Slash16 max CPU share"
		userAgentShareHelp     = "UserAgent max CPU share"
		hashMaxLenHelp         = "Maximum amount of entries in the hash"
	)
	tw := tabwriter.NewWriter(os.Stdout, 24, 4, 1, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "Value\t   Option\f")
	fmt.Fprintf(tw, "%t\t - %s\f", conf.Debug, debugHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.LowCreditThreshold*100), lowCreditThresholdHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.Backend, backendHelp)
	fmt.Fprintf(tw, "%d\t - %s\f", conf.ListenPort, listenPortHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.LogFile, logFileHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.StatsFile, statsFileHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.Graphite, graphiteHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.GraphitePrefix, graphitePrefixHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.TrustedProxies, trustedProxiesHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.Slash32Share*100.0), slash32ShareHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.Slash24Share*100.0), slash24ShareHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.Slash16Share*100.0), slash16ShareHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.UserAgentShare*100.0), userAgentShareHelp)
	fmt.Fprintf(tw, "%d\t - %s\f", conf.HashMaxLen, hashMaxLenHelp)
}

func middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		Slash32.Register(r, cost)
		Slash24.Register(r, cost)
		Slash16.Register(r, cost)
		UserAgent.Register(r, cost)
	})
}

func statsLogger() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		Slash32.Dump(statsLog)
		Slash24.Dump(statsLog)
		Slash16.Dump(statsLog)
		UserAgent.Dump(statsLog)

		sendMetric("Slash32", fmt.Sprintf("%d", Slash32.CountUnderThreshold()))
		sendMetric("Slash24", fmt.Sprintf("%d", Slash24.CountUnderThreshold()))
		sendMetric("Slash16", fmt.Sprintf("%d", Slash16.CountUnderThreshold()))
		sendMetric("UserAgent", fmt.Sprintf("%d", UserAgent.CountUnderThreshold()))
	}
}

type BucketDumper struct {
}

func (bd *BucketDumper) Slash32(args *api.EmptyArgs, reply *bucket.DumpList) error {
	*reply = Slash32.DumpList()
	return nil
}

func (bd *BucketDumper) Slash24(args *api.EmptyArgs, reply *bucket.DumpList) error {
	*reply = Slash24.DumpList()
	return nil
}

func (bd *BucketDumper) Slash16(args *api.EmptyArgs, reply *bucket.DumpList) error {
	*reply = Slash16.DumpList()
	return nil
}

func (bd *BucketDumper) UserAgent(args *api.EmptyArgs, reply *bucket.DumpList) error {
	*reply = UserAgent.DumpList()
	return nil
}

func listen() {
	url, err := url.Parse(conf.Backend)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	handler := middleware(proxy)
	http.ListenAndServe(fmt.Sprintf(":%d", conf.ListenPort), handler)
}

func sighupHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	for {
		<-c
		log.Warn("SIGHUP received, reloading config")

		newConfig := newConfig()
		jsonStr, err := ioutil.ReadFile("/etc/tempomat.json")
		if err != nil {
			log.Error(err)
			log.Error("Refusing to reload on unreadable config file.")
		}
		if err = json.Unmarshal(jsonStr, &newConfig); err != nil {
			log.Error(err)
			log.Error("Refusing to reload on unparseable config file.")
		}

		if newConfig.LowCreditThreshold != conf.LowCreditThreshold {
			conf.LowCreditThreshold = newConfig.LowCreditThreshold
			Slash32.SetLowCreditThreshold(conf.LowCreditThreshold)
			Slash24.SetLowCreditThreshold(conf.LowCreditThreshold)
			Slash16.SetLowCreditThreshold(conf.LowCreditThreshold)
			UserAgent.SetLowCreditThreshold(conf.LowCreditThreshold)
		}

		if newConfig.Slash32Share != conf.Slash32Share {
			conf.Slash32Share = newConfig.Slash32Share
			Slash32.SetRate(conf.Slash32Share)
		}
		if newConfig.Slash24Share != conf.Slash24Share {
			conf.Slash24Share = newConfig.Slash24Share
			Slash24.SetRate(conf.Slash24Share)
		}
		if newConfig.Slash16Share != conf.Slash16Share {
			conf.Slash16Share = newConfig.Slash16Share
			Slash16.SetRate(conf.Slash16Share)
		}
		if newConfig.UserAgentShare != conf.UserAgentShare {
			conf.UserAgentShare = newConfig.UserAgentShare
			UserAgent.SetRate(conf.UserAgentShare)
		}
		if newConfig.HashMaxLen != conf.HashMaxLen {
			conf.HashMaxLen = newConfig.HashMaxLen
			Slash32.SetHashMaxLen(conf.HashMaxLen)
			Slash24.SetHashMaxLen(conf.HashMaxLen)
			Slash16.SetHashMaxLen(conf.HashMaxLen)
			UserAgent.SetHashMaxLen(conf.HashMaxLen)
		}

		if conf.Debug {
			printConf()
		}
	}
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	go sighupHandler()
	go statsLogger()

	bd := new(BucketDumper)
	rpc.Register(bd)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":29999")
	if e != nil {
		log.Fatal("Unable to set up RPC listener:", e)
	}
	go http.Serve(l, nil)

	listen()
}
