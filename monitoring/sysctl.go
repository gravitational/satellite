/*
Copyright 2017 Gravitational, Inc.

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
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"
)

// Sysctl returns kernel parameter by reading proc/sys
func Sysctl(name string) (string, error) {
	path := filepath.Clean(filepath.Join("proc", "sys", strings.Replace(name, ".", "/", -1)))
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}
	return string(data[:len(data)-1]), nil
}
