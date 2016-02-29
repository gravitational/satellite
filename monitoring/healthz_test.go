package monitoring

import (
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
	checker.Check(&reporter)

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
	checker.Check(&reporter)

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
	checker.Check(&reporter)

	c.Assert(reporter, HasLen, 1)
	c.Assert(reporter[0].Error, Matches, `.*Client\.Timeout.*`)
}
