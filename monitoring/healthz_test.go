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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"

	. "gopkg.in/check.v1"
)

func TestHealthz(t *testing.T) { TestingT(t) }

type HealthzSuite struct{}

var _ = Suite(&HealthzSuite{})

func (_ *HealthzSuite) TestSetsCode(c *C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprintln(w, "Internal Server Error")
	}))
	defer srv.Close()
	checker := NewHTTPHealthzChecker("foo", srv.URL, func(resp io.Reader) error {
		return nil
	})
	var reporter health.Probes
	checker.Check(context.TODO(), &reporter)

	c.Assert(reporter, HasLen, 1)
	c.Assert(reporter[0].Code, Equals, "500")
}

func (_ *HealthzSuite) TestObtainsSuccessProbe(c *C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintln(w, "ok")
	}))
	defer srv.Close()
	checker := NewHTTPHealthzChecker("foo", srv.URL, func(resp io.Reader) error {
		return nil
	})
	var reporter health.Probes
	checker.Check(context.TODO(), &reporter)

	expectedProbes := health.Probes{
		{
			Checker: "foo",
			Status:  pb.Probe_Running,
		},
	}
	c.Assert(reporter, DeepEquals, expectedProbes)
}

func (_ *HealthzSuite) TestUsesClientTimeout(c *C) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response time is too long
		time.Sleep(healthzCheckTimeout * 2)
	}))
	defer srv.Close()
	checker := NewHTTPHealthzChecker("foo", srv.URL, func(resp io.Reader) error {
		return nil
	})
	var reporter health.Probes
	checker.Check(context.TODO(), &reporter)

	c.Assert(reporter, HasLen, 1)
	c.Assert(reporter[0].Error, Matches, `.*Client\.Timeout.*`)
}
