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
	"context"
	"io"
	"io/ioutil"
	"os/exec"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/trace"
)

// NewScriptChecker returns a new script checker for the specified script
func NewScriptChecker(script io.Reader, args ...string) health.Checker {
	return scriptChecker{Reader: script, args: args}
}

// Name returns name of the checker.
// Implements health.Checker
func (r scriptChecker) Name() string {
	return scriptCheckerID
}

func (r scriptChecker) Check(ctx context.Context, reporter health.Reporter) {
	f, err := ioutil.TempFile("", "monitoring")
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), "failed to create script",
			trace.ConvertSystemError(err)))
		return
	}
	defer f.Close()

	_, err = io.Copy(f, r.Reader)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), "failed to write script file",
			trace.Wrap(err)))
		return
	}

	args := append([]string{f.Name()}, r.args...)
	cmd := exec.Command("bash", args...)
	buf, err := cmd.CombinedOutput()
	if err != nil {
		reporter.Add(NewProbeFromErr(r.Name(), string(buf),
			trace.Wrap(err)))
		return
	}

	reporter.Add(NewSuccessProbe(r.Name()))
}

// scriptChecker is a checker that executes the specified script
type scriptChecker struct {
	io.Reader
	args []string
}

const scriptCheckerID = "script-check"
