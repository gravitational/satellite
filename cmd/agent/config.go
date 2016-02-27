package main

import (
	"fmt"

	kv "github.com/gravitational/configure"
	"github.com/gravitational/configure/cstrings"

	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	EnvPublicIP       = "MONITOR_PUBLIC_IP"
	EnvRole           = "MONITOR_AGENT_ROLE"
	EnvAgentName      = "MONITOR_AGENT_NAME"
	EnvInitialCluster = "MONITOR_INITIAL_CLUSTER"
	EnvStateDir       = "MONITOR_STATE_DIR"
	EnvTags           = "MONITOR_TAGS"

	EnvInfluxDatabase = "SATELLITE_INFLUX_DATABASE"
	EnvInfluxUser     = "SATELLITE_INFLUX_USER"
	EnvInfluxPassword = "SATELLITE_INFLUX_PASSWORD"
	EnvInfluxURL      = "SATELLITE_INFLUX_URL"
)

func ListFlag(s kingpin.Settings) (result *paramList) {
	result = new(paramList)
	s.SetValue(result)
	return result
}

func KeyValueListFlag(s kingpin.Settings) (result *kv.KeyVal) {
	result = new(kv.KeyVal)
	s.SetValue(result)
	return result
}

type paramList []string

func (r *paramList) Set(value string) error {
	for _, param := range cstrings.SplitComma(value) {
		*r = append(*r, param)
	}
	return nil
}

func (r *paramList) String() string {
	return fmt.Sprintf("%v", []string(*r))
}

// toAddrList interprets each key/value as domain=address and extracts
// just the address part.
func toAddrList(store kv.KeyVal) (addrs []string) {
	for _, addr := range store {
		addrs = append(addrs, addr)
	}
	return addrs
}
