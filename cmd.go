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
	"log"
)

var conf config.Config

var buckets []bucket.Bucketable

var stdoutLog *log.Logger
var stderrLog *log.Logger

func init() {
	stdoutLog = log.New(os.Stdout, "", 0)
	stderrLog = log.New(os.Stderr, "", 0)

	var err error
	conf, err = config.NewConfig()
	if err!=nil {
		stderrLog.Printf("%s\n", err)
		os.Exit(1)
	}

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
		conf.Print(stdoutLog)
	}

	buckets = append(buckets, bucket.NewSlash32(conf, 32))
	buckets = append(buckets, bucket.NewSlash32(conf, 24))
	buckets = append(buckets, bucket.NewSlash32(conf, 16))
	buckets = append(buckets, bucket.NewUserAgent(conf))
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
	if conf.Graphite=="" {
		return
	}

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
		start := time.Now()
		proxy.ServeHTTP(w, r)
		// TODO I'm not sure if this expresses the time from beginning till the end. Reqs
		// that take 3s to complete (because of load) register as 0.7 here...
		reqTime := time.Since(start)

		// Cost is expressed in the amount of compute seconds consumed.
		// It's scaled down by load average to certain degree if the CPU is 100% saturated to take CPU contention into account.
		cost := float64(reqTime) / float64(time.Second)

		var maxDelay time.Duration
		header := 200
		for _, b := range buckets {
			// TODO
			// be very very careful of not reading the request.Body unless copying it before.
			// The Body is a io.ReadClose so if you read it, it will be empty (closed) for other calls to it,
			// @see https://medium.com/@xoen/golang-read-from-an-io-readwriter-without-loosing-its-content-2c6911805361
			// I'm not sure how this works with the reverseProxy functionality
			// wouldn't it be great if we could mark stuff as immutable.
			bucketDelay, ok := b.ReserveN(r, start, cost)
			if !ok {
				header = 503
			}
			if (bucketDelay>maxDelay) {
				maxDelay = bucketDelay
			}
		}

		if header!=200 {
			w.WriteHeader(header)
		}
		holdCaller(start, maxDelay)
	})
}

func holdCaller(start time.Time, delay time.Duration) {
	elapsed := time.Now().Sub(start)
	if elapsed>=delay {
		return
	}

	time.Sleep(delay - elapsed)
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

		newConfig, err := config.NewConfig()
		if err!=nil {

		}
		jsonStr, err := ioutil.ReadFile("/etc/tempomat.json")
		if err != nil {
			stderrLog.Printf("Unreadable config file: %s\n", err)
		}
		if err = json.Unmarshal(jsonStr, &newConfig); err != nil {
			stderrLog.Printf("Unparseable config file: %s\n", err)
			return
		}

		conf = newConfig

		for _, b := range buckets {
			b.SetConfig(conf)
		}

		if conf.Debug {
			conf.Print(stdoutLog)
		}
	}
}

func main() {
	go func() {
		stdoutLog.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	go sighupHandler()
	go statsLogger()

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
