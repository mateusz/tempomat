package config

import (
	"net/url"
	"io/ioutil"
	"encoding/json"
	"strings"
	"github.com/shirou/gopsutil/cpu"
	"log"
)

type Config struct {
	Debug             bool            `json:"debug"`
	DelayThresholdSec float64         `json:"delayThresholdSec"`
	Backend           string          `json:"backend"`
	ListenPort        int             `json:"listenPort"`
	Graphite          string          `json:"graphite"`
	GraphitePrefix    string          `json:"graphitePrefix"`
	TrustedProxies    string          `json:"trustedProxies"`
	CPUCount	  float64	  `json:"cpuCount"`
	Slash32Share      float64         `json:"slash32Share"`
	Slash24Share      float64         `json:"slash24Share"`
	Slash16Share      float64         `json:"slash16Share"`
	UserAgentShare    float64         `json:"userAgentShare"`
	Slash32CPUs       float64         `json:"-"`
	Slash24CPUs       float64         `json:"-"`
	Slash16CPUs       float64         `json:"-"`
	UserAgentCPUs     float64         `json:"-"`
	HashMaxLen        int             `json:"hashMaxLen"`
	GraphiteURL       *url.URL        `json:"-"`
	TrustedProxiesMap map[string]bool `json:"-"`
}

func NewConfig() (Config, error) {

	conf := Config{
		Debug:              false,
		DelayThresholdSec:  10,
		Backend:            "http://localhost:80",
		ListenPort:         8888,
		Graphite:           "",
		GraphitePrefix:     "",
		TrustedProxies:     "",
		HashMaxLen:         1000,
		GraphiteURL:        nil,
		TrustedProxiesMap:  make(map[string]bool),
	}

	jsonStr, err := ioutil.ReadFile("/etc/tempomat.json")
	if err != nil {
		return Config{}, err
	}
	if err = json.Unmarshal(jsonStr, &conf); err != nil {
		return Config{}, err
	}

	var cpuCount float64
	if conf.CPUCount==0 {
		cpuCountInt, err := cpu.Counts(true)
		if err != nil {
			return Config{}, err
		}
		cpuCount = float64(cpuCountInt)
	} else {
		cpuCount = conf.CPUCount
	}

	// Defaults
	conf.Slash32CPUs = 0.5 * cpuCount
	conf.Slash24CPUs = 0.5 * cpuCount
	conf.Slash16CPUs = 0.5 * cpuCount
	conf.UserAgentCPUs = 0.5 * cpuCount

	if conf.Slash32Share !=0 {
		conf.Slash32CPUs = conf.Slash32Share * cpuCount
	}
	if conf.Slash24Share !=0 {
		conf.Slash24CPUs = conf.Slash24Share * cpuCount
	}
	if conf.Slash16Share !=0 {
		conf.Slash16CPUs = conf.Slash16Share * cpuCount
	}
	if conf.UserAgentShare !=0 {
		conf.UserAgentCPUs = conf.UserAgentShare * cpuCount
	}

	proxies := strings.Split(conf.TrustedProxies, ",")
	for _, proxy := range proxies {
		conf.TrustedProxiesMap[proxy] = true
	}

	return conf, nil
}

func (conf *Config) Print(log *log.Logger) {
	log.Print("GENERAL")
	log.Printf("Debug mode:         %t", conf.Debug)
	log.Printf("Backend URI:        %s", conf.Backend)
	log.Printf("Local listen port:  %d", conf.ListenPort)
	log.Printf("Trusted proxy ips:  '%s'", conf.TrustedProxies)
	log.Printf("Maximum hash size:  %d", conf.HashMaxLen)
	log.Print("")
	log.Print("STATS")
	log.Printf("Graphite server:    '%s' (e.g. 'tcp://localhost:2003')", conf.Graphite)
	log.Printf("Graphite prefix:    '%s' (e.g. 'chaos.schmall.prod')", conf.GraphitePrefix)
	log.Printf("Stats delay thresh: %.3fs", conf.DelayThresholdSec)
	log.Print("")
	log.Print("WEIGHTS")
	log.Printf("Explicit CPU count:               %.2f", conf.CPUCount)
	log.Printf("Slash32 max CPU share:            %d%%", int(conf.Slash32Share *100.0))
	log.Printf("Slash24 max CPU share:            %d%%", int(conf.Slash24Share *100.0))
	log.Printf("Slash16 max CPU share:            %d%%", int(conf.Slash16Share *100.0))
	log.Printf("UserAgent max CPU share:          %d%%", int(conf.UserAgentShare *100.0))
	log.Print("")
	log.Print("COMPUTED")
	log.Printf("Slash32 max CPU absolute usage:   %.2fcpus", conf.Slash32CPUs)
	log.Printf("Slash24 max CPU absolute usage:   %.2fcpus", conf.Slash24CPUs)
	log.Printf("Slash16 max CPU absolute usage:   %.2fcpus", conf.Slash16CPUs)
	log.Printf("UserAgent max CPU absolute usage: %.2fcpus", conf.UserAgentCPUs)
}
