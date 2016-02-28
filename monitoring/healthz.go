package monitoring

import (
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

const healthzCheckTimeout = 1 * time.Second

// HttpResponseChecker is a function that can validate service status from response
type HttpResponseChecker func(response io.Reader) error

// HttpHealthzChecker is a health checker that can check servhice status over HTTP
type HttpHealthzChecker struct {
	name    string
	URL     string
	client  *http.Client
	checker HttpResponseChecker
}

// Name returns the name of this checker
func (r *HttpHealthzChecker) Name() string { return r.name }

// Check runs an HTTP check and reports errors to the specified Reporter
func (r *HttpHealthzChecker) Check(reporter health.Reporter) {
	resp, err := r.client.Get(r.URL)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.name, trace.Errorf("healthz check failed: %v", err)))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reporter.Add(NewProbeFromErr(r.name, trace.Errorf("unexpected HTTP status: %s", http.StatusText(resp.StatusCode))))
		return
	}
	if err = r.checker(resp.Body); err != nil {
		reporter.Add(NewProbeFromErr(r.name, err))
		return
	}
	reporter.Add(&pb.Probe{
		Checker: r.name,
		Status:  pb.Probe_Running,
	})
}

// NewHTTPHealthzChecker is a Checker that tests the specified HTTP endpoint
func NewHTTPHealthzChecker(name, URL string, checker HttpResponseChecker) health.Checker {
	client := &http.Client{
		Timeout: healthzCheckTimeout,
	}
	return &HttpHealthzChecker{
		name:    name,
		URL:     URL,
		client:  client,
		checker: checker,
	}
}

// NewUnixSocketHealthzChecker returns a new Checker that tests
// the specified unix domain socket path and URL
func NewUnixSocketHealthzChecker(name, URL, socketPath string, checker HttpResponseChecker) health.Checker {
	client := &http.Client{
		Timeout: healthzCheckTimeout,
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
	return &HttpHealthzChecker{
		name:    name,
		URL:     URL,
		client:  client,
		checker: checker,
	}
}
