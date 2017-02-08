/*
Copyright 2016 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// ListFlag defines a command line flag that can accumulate multiple string values
func ListFlag(s kingpin.Settings) (result *stringList) {
	result = new(stringList)
	s.SetValue(result)
	return result
}

// KeyValueListFlag defines a command line flag that can accumulate multiple key/value pairs
func KeyValueListFlag(s kingpin.Settings) (result *kv.KeyVal) {
	result = new(kv.KeyVal)
	s.SetValue(result)
	return result
}

// StringList creates a command line flag that interprets comma-separated list of values
func StringList(s kingpin.Settings) (result *stringList) {
	result = new(stringList)
	s.SetValue(result)
	return result
}

// stringList defines a command line flag that interprets comma-separated list of values
type stringList []string

// Set splits a comma-separated string value into a list of strings
func (r *stringList) Set(value string) error {
	for _, item := range cstrings.SplitComma(value) {
		*r = append(*r, item)
	}
	return nil
}

func (r *stringList) String() string {
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
