package config

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	envDebug      = "DEBUG"
	envThresholds = "HEALTH_THRESHOLDS"
	envAccessKey  = "HEALTH_ACCESSKEY"
	envListen     = "HEALTH_LISTEN"
	envInterval   = "HEALTH_INTERVAL"
	envLogLevel   = "HEALTH_LOGLEVEL"
	envCertFile   = "HEALTH_CERTFILE"
	envCAFile     = "HEALTH_CAFILE"
	envKeyFile    = "HEALTH_KEYFILE"
)

const (
	docThresholds = "Specify thresholds for amount of unavailable nodes in cluster." +
		"Format is `0=Y,Z=U`. It does mean that for cluster with size from " +
		"0 to Z (excluded) nodes check will fail if more than Y% of nodes is " +
		"unavailable. And for cluster with size bigger than Z nodes it will fail if " +
		"more than U% nodes is unavailable. Thresholds MUST be specified starting with 0."
	docAccessKey = "Key to access health-check url. Must be supplied as `accesskey` " +
		"query parameter either in POST (recommended) or in GET query string."
)

const (
	flagDebug      = "debug"
	flagThresholds = "thresholds"
	flagAccessKey  = "access-key"
	flagListen     = "listen"
	flagInterval   = "interval"
	flagLogLevel   = "log-level"
	flagCertFile   = "cert-file"
	flagKeyFile    = "key-file"
	flagCAFile     = "ca-file"
)

type key int

const (
	contextKey key = iota
)

// Config stores application general configuration
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

// AttachToContext adds *Config to context
func AttachToContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey, cfg)
}

// FromContext tries to get *Config from context
func FromContext(ctx context.Context) (*Config, bool) {
	cfg, ok := ctx.Value(contextKey).(*Config)
	return cfg, ok
}

// ParseCLIFlags parses and stores supplied command flags into config structure
func ParseCLIFlags(c *Config) error {
	thresholds := kingpin.Flag(flagThresholds, docThresholds).Default("0:30,5:20").Envar(envThresholds).String()
	debug := kingpin.Flag(flagDebug, "Turn on debug mode").Default("").Envar(envDebug).String()
	kingpin.Flag(flagAccessKey, docAccessKey).Required().Envar(envAccessKey).StringVar(&c.AccessKey)
	kingpin.Flag(flagListen, "Address to listen on.").Default("0.0.0.0:8080").Envar(envListen).StringVar(&c.Listen)
	kingpin.Flag(flagInterval, "Interval between checks.").Default("1m").Envar(envInterval).DurationVar(&c.Interval)
	kingpin.Flag(flagLogLevel, "Log level.").Default("info").Envar(envLogLevel).EnumVar(&c.LogLevel, "debug", "info", "warning", "error", "fatal", "panic")
	kingpin.Flag(flagCertFile, "Path to the client server TLS cert file.").Envar(envCertFile).StringVar(&c.CertFile)
	kingpin.Flag(flagKeyFile, "Path to the client server TLS key file.").Envar(envKeyFile).StringVar(&c.KeyFile)
	kingpin.Flag(flagCAFile, "Path to the client server TLS CA file.").Envar(envCAFile).StringVar(&c.CAFile)
	kingpin.Parse()

	if *debug != "" {
		c.LogLevel = "debug"
	}

	var err error
	c.Thresholds, err = NewThresholds(*thresholds)
	if err != nil {
		return trace.Errorf("unable to parse threshold value: %s", *thresholds)
	}
	return nil
}
