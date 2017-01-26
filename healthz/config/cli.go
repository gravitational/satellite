package config

import (
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/gravitational/trace"
)

const (
	EnvThresholds = "HEALTH_THRESHOLDS"
	EnvAccessKey = "HEALTH_ACCESSKEY"
	EnvListen = "HEALTH_LISTEN"
	EnvInterval = "HEALTH_INTERVAL"
	EnvLogLevel = "HEALTH_LOGLEVEL"
	EnvCertFile = "HEALTH_CERTFILE"
	EnvCAFile = "HEALTH_CAFILE"
	EnvKeyFile = "HEALTH_KEYFILE"
)

const (
	DocThresholds = "Specify thresholds for amount of dead nodes in cluster." +
			"Format is `0=Y,Z=U`. It does mean that for cluster with size from " +
			"0 to Z (excluded) nodes check will fail if more than Y% of nodes is " +
			"dead. And for cluster with size bigger than Z nodes it will fail if " +
			"more than U% nodes is dead. Thresholds MUST be specified starting with 0."
	DocAccessKey = "Key to access health-check url. Must be supplied as `accesskey` " +
			"query parameter either in POST (recommended) or in GET query string."
)

const (
	FlagThresholds = "thresholds"
	FlagAccessKey = "access-key"
	FlagListen = "listen"
	FlagInterval = "interval"
	FlagLogLevel = "log-level"
	FlagCertFile = "cert-file"
	FlagKeyFile = "key-file"
	FlagCAFile = "ca-file"
)

const (
	ContextKey = "config"
)

type Config struct {
	AccessKey  string
	Listen     string
	Interval   time.Duration
	LogLevel   string
	CertFile   string
	CAFile     string
	KeyFile    string
	Thresholds *Thresholds
}

func ParseCLIFlags(c *Config) {
	thresholds := kingpin.Flag(FlagThresholds, DocThresholds).Default("0:30,5:20").Envar(EnvThresholds).String()
	kingpin.Flag(FlagAccessKey, DocAccessKey).Required().Envar(EnvAccessKey).StringVar(&c.AccessKey)
	kingpin.Flag(FlagListen, "Address to listen on.").Default("0.0.0.0:8080").Envar(EnvListen).StringVar(&c.Listen)
	kingpin.Flag(FlagInterval, "Interval between checks.").Default("1m").Envar(EnvInterval).DurationVar(&c.Interval)
	kingpin.Flag(FlagLogLevel, "Log level.").Default("info").Envar(EnvLogLevel).EnumVar(&c.LogLevel, "debug", "info", "warning", "error", "fatal", "panic")
	kingpin.Flag(FlagCertFile, "Path to the client server TLS cert file.").Envar(EnvCertFile).StringVar(&c.CertFile)
	kingpin.Flag(FlagKeyFile, "Path to the client server TLS key file.").Envar(EnvKeyFile).StringVar(&c.KeyFile)
	kingpin.Flag(FlagCAFile, "Path to the client server TLS CA file.").Envar(EnvCAFile).StringVar(&c.CAFile)
	kingpin.Parse()

	var err error
	c.Thresholds, err = NewThresholds(*thresholds)
	if err != nil {
		trace.Fatalf("Unable to parse threshold value: %s", *thresholds)
	}
}
