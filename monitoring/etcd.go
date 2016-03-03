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
package monitoring

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"time"

	"github.com/gravitational/trace"
)

// defaultDialTimeout is the maximum amount of time a dial will wait for a connection to setup.
const defaultDialTimeout = 30 * time.Second

// etcdChecker is an HttpResponseChecker that interprets results from
// an etcd HTTP-based healthz end-point.
func etcdChecker(response io.Reader) error {
	payload, err := ioutil.ReadAll(response)
	if err != nil {
		return trace.Wrap(err)
	}

	healthy, err := etcdStatus(payload)
	if err != nil {
		return trace.Wrap(err)
	}

	if !healthy {
		return trace.Errorf("unexpected etcd status: %s", payload)
	}
	return nil
}

func etcdStatus(payload []byte) (healthy bool, err error) {
	result := struct{ Health string }{}
	nresult := struct{ Health bool }{}
	err = json.Unmarshal(payload, &result)
	if err != nil {
		err = json.Unmarshal(payload, &nresult)
	}
	if err != nil {
		return false, trace.Wrap(err)
	}

	return (result.Health == "true" || nresult.Health == true), nil
}
