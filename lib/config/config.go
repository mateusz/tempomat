package config

import (
	"fmt"
	"net/url"
	"os"
	"text/tabwriter"
)

type Config struct {
	Debug             bool            `json:"debug"`
	DelayThresholdSec float64         `json:"delayThresholdSec"`
	Backend           string          `json:"backend"`
	ListenPort        int             `json:"listenPort"`
	Graphite          string          `json:"graphite"`
	GraphitePrefix    string          `json:"graphitePrefix"`
	TrustedProxies    string          `json:"trustedProxies"`
	Slash32Share      float64         `json:"slash32Share"`
	Slash24Share      float64         `json:"slash24Share"`
	Slash16Share      float64         `json:"slash16Share"`
	UserAgentShare    float64         `json:"userAgentShare"`
	HashMaxLen        int             `json:"hashMaxLen"`
	GraphiteURL       *url.URL        `json:"-"`
	TrustedProxiesMap map[string]bool `json:"-"`
}

func New() Config {
	return Config{
		Debug:              false,
		DelayThresholdSec:  10,
		Backend:            "http://localhost:80",
		ListenPort:         8888,
		Graphite:           "",
		GraphitePrefix:     "",
		TrustedProxies:     "",
		Slash32Share:       0.1,
		Slash24Share:       0.25,
		Slash16Share:       0.5,
		UserAgentShare:     0.1,
		HashMaxLen:         1000,
		GraphiteURL:        nil,
		TrustedProxiesMap:  make(map[string]bool),
	}
}

func (conf *Config) Print() {
	const (
		debugHelp              = "Debug mode"
		delayThresholdSecHelp  = "Delay threshold above which to include in stats (in seconds)"
		backendHelp            = "Backend URI"
		listenPortHelp         = "Local HTTP listen port"
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
	fmt.Fprint(tw, "Value\t   Option\f")
	fmt.Fprintf(tw, "%t\t - %s\f", conf.Debug, debugHelp)
	fmt.Fprintf(tw, "%.3fs\t - %s\f", conf.DelayThresholdSec, delayThresholdSecHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.Backend, backendHelp)
	fmt.Fprintf(tw, "%d\t - %s\f", conf.ListenPort, listenPortHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.Graphite, graphiteHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.GraphitePrefix, graphitePrefixHelp)
	fmt.Fprintf(tw, "%s\t - %s\f", conf.TrustedProxies, trustedProxiesHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.Slash32Share*100.0), slash32ShareHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.Slash24Share*100.0), slash24ShareHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.Slash16Share*100.0), slash16ShareHelp)
	fmt.Fprintf(tw, "%d%%\t - %s\f", int(conf.UserAgentShare*100.0), userAgentShareHelp)
	fmt.Fprintf(tw, "%d\t - %s\f", conf.HashMaxLen, hashMaxLenHelp)
}
