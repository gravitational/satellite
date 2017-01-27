package config

import (
	"time"

	"github.com/gravitational/satellite/monitoring"
)

// Config stores application configuration
type Config struct {
	Debug         bool
	ListenAddr    string
	CheckInterval time.Duration
	AccessKey     string
	KubeAddr      string
	ETCDConfig    monitoring.ETCDConfig
	CertFile      string
	KeyFile       string
	CAFile        string
}
