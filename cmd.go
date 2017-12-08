package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"net/rpc"

	"github.com/mateusz/tempomat/api"
	"github.com/mateusz/tempomat/bucket"
	"github.com/mateusz/tempomat/lib/config"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/load"
	"golang.org/x/time/rate"
	"log"
)

var conf config.Config

var buckets []bucket.Bucketable

var cpuCount float64

var stdoutLog *log.Logger
var stderrLog *log.Logger

func utilisation() (float64, error) {
	// beware that the variable name load collides with the package load
	load, err := load.Avg()
	if err != nil {
		return -1, err
	}

	return load.Load1 / cpuCount, nil
}

func init() {
	stdoutLog = log.New(os.Stdout, "", 0)
	stderrLog = log.New(os.Stderr, "", 0)

	conf = config.New()

	jsonStr, err := ioutil.ReadFile("/etc/tempomat.json")
	if err != nil {
		stderrLog.Printf("Unreadable config file: %s\n", err)
		os.Exit(1)
	}
	if err = json.Unmarshal(jsonStr, &conf); err != nil {
		stderrLog.Printf("Unparseable config file: %s\n", err)
		os.Exit(1)
	}

	proxies := strings.Split(conf.TrustedProxies, ",")
	for _, proxy := range proxies {
		conf.TrustedProxiesMap[proxy] = true
	}

	// TODO allow overriding, or maybe just switch "Shares" to absolute values? (i.e. 1.0cpu instead of 12.5%)
	cpuCountInt, err := cpu.Counts(true)
	if err != nil {
		stderrLog.Printf("%s\n", err)
		os.Exit(1)
	}
	cpuCount = float64(cpuCountInt)

	if conf.Graphite != "" {
		if conf.GraphitePrefix == "" {
			stderrLog.Print("Configuration failure: 'graphitePrefix' is required if 'graphite' is specified\n")
			os.Exit(1)
		}
		hostname, errHostname := os.Hostname()
		if errHostname != nil {
			stderrLog.Printf("%s\n", errHostname)
			os.Exit(1)
		}
		conf.GraphitePrefix = strings.Replace(conf.GraphitePrefix, "{hostname}", hostname, -1)

		var errParse error
		conf.GraphiteURL, errParse = url.Parse(conf.Graphite)
		if errParse != nil {
			stderrLog.Printf("%s\n", errParse)
			os.Exit(1)
		}
	}

	if conf.Debug {
		conf.Print()
	}

	buckets = append(buckets, bucket.NewSlash32(conf, cpuCount, 32))
	buckets = append(buckets, bucket.NewSlash32(conf, cpuCount, 24))
	buckets = append(buckets, bucket.NewSlash32(conf, cpuCount, 16))
	buckets = append(buckets, bucket.NewUserAgent(conf, cpuCount))
}

func statsLogger() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		for _, b := range buckets {
			for _, e := range b.Entries() {
				if e.AvgWait() > b.DelayThreshold() {
					stdoutLog.Printf("%s,'%s',%.2f", b, e.Title(), e.AvgWait().Seconds())
				}
			}
			sendMetric(b.String(), fmt.Sprintf("%d", CountOverThreshold(b)))
		}
	}
}

func sendMetric(metric string, value string) {
	dialer, err := net.Dial("tcp", conf.GraphiteURL.Host)
	if err != nil {
		stderrLog.Printf("Failed to connect to graphite server: %s", err)
		return
	}

	_, err = dialer.Write([]byte(fmt.Sprintf("%s.%s %s %d\n", conf.GraphitePrefix, metric, value, time.Now().Unix())))
	if err != nil {
		stderrLog.Printf("Failed to write to graphite server: %s", err)
	}
	if err = dialer.Close(); err != nil {
		stderrLog.Printf("Failed to Close() connection to graphite server: %s", err)
	}
}

func middleware(proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO pre-check the token bucket before forwarding the request. Can we fail early?
		start := time.Now()
		proxy.ServeHTTP(w, r)
		// TODO I'm not sure if this expresses the time from beginning till the end. Reqs
		// that take 3s to complete (because of load) register as 0.7 here...
		reqTime := time.Since(start)

		u, err := utilisation()
		if err != nil {
			stderrLog.Printf("%s, assuming utilisation=1.0", err)
			u = 1.0
		}

		// Cost is expressed in the amount of compute seconds consumed.
		// It's scaled down by load average to certain degree if the CPU is 100% saturated to take CPU contention into account.
		cost := float64(reqTime) / float64(time.Second)
		// TODO maybe get rid of scaling - so this can be deployed on a different server. It seems to introduce instability anyway.
		if u > 1.0 {
			cost = cost / (0.5*u)
		}
		fmt.Printf("%.2f\n", cost)

		var maxDelay time.Duration
		for _, b := range buckets {
			// be very very careful of not reading the request.Body unless copying it before.
			// The Body is a io.ReadClose so if you read it, it will be empty (closed) for other calls to it,
			// @see https://medium.com/@xoen/golang-read-from-an-io-readwriter-without-loosing-its-content-2c6911805361
			// I'm not sure how this works with the reverseProxy functionality
			// wouldn't it be great if we could mark stuff as immutable.
			rsv := b.ReserveN(r, start, cost)
			if !rsv.OK() {
				w.WriteHeader(503)
				// Hold the client up for a bit as a penalty
				time.Sleep(60 * time.Second)
				// TODO How often does this trigger?
				fmt.Print("DROP\n")
				return
			}
			bucketDelay := rsv.Delay();
			if (bucketDelay>maxDelay) {
				maxDelay = bucketDelay
			}
		}

		if maxDelay==rate.InfDuration {
			w.WriteHeader(503)
			// Hold the client up for a bit as a penalty
			// TODO How often does this trigger?
			fmt.Print("DROP\n")
			time.Sleep(60 * time.Second)
		} else {
			time.Sleep(maxDelay)
		}
	})
}

func listen() {
	// beware that url is colliding with the imported package url
	addr, err := url.Parse(conf.Backend)
	if err != nil {
		stderrLog.Printf("%s\n", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(addr)
	handler := middleware(proxy)
	http.ListenAndServe(fmt.Sprintf(":%d", conf.ListenPort), handler)
}

func sighupHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	for {
		<-c
		stdoutLog.Print("SIGHUP received, reloading config\n")

		newConfig := config.New()
		jsonStr, err := ioutil.ReadFile("/etc/tempomat.json")
		if err != nil {
			stderrLog.Printf("Unreadable config file: %s\n", err)
		}
		if err = json.Unmarshal(jsonStr, &newConfig); err != nil {
			stderrLog.Printf("Unparseable config file: %s\n", err)
			return
		}

		// These changes must all be secured for concurrent access.
		// For example changing Debug is not safe, because it results in data race.
		// TODO these won't work without reprovisioning the buckets
		conf.UserAgentShare = newConfig.UserAgentShare
		conf.Slash32Share = newConfig.Slash32Share
		conf.Slash24Share = newConfig.Slash24Share
		conf.Slash16Share = newConfig.Slash16Share
		conf.DelayThresholdSec = newConfig.DelayThresholdSec

		for _, b := range buckets {
			b.SetConfig(conf)
		}

		if conf.Debug {
			conf.Print()
		}
	}
}

func main() {
	go func() {
		stdoutLog.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	go sighupHandler()
	go statsLogger()

	// @todo: need to figure this out
	rpc.Register(api.NewTempomatAPI(buckets))
	rpc.HandleHTTP()
	l, err := net.Listen("tcp", ":29999")
	if err != nil {
		stderrLog.Printf("Unable to set up RPC listener: %s\n", err)
		os.Exit(1)
	}
	go http.Serve(l, nil)

	listen()
}

func CountOverThreshold(b bucket.Bucketable) int {
	var count int
	for _, e := range b.Entries() {
		if e.AvgWait() > b.DelayThreshold() {
			count++
		}
	}
	return count
}
