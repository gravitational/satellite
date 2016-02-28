package monitoring

import (
	"io"
	"net"
	"net/http"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"
)

const healthzCheckTimeout = 1 * time.Second

// checkerFunc is a function that can read service health status and report if it has failed.
type checkerFunc func(response io.Reader) error

// httpHealthzChecker is a health checker that probes service status using
// an HTTP healthz end-point.
type httpHealthzChecker struct {
	URL         string
	client      *http.Client
	checkerFunc checkerFunc
}

func (r *httpHealthzChecker) check(reporter reporter) {
	resp, err := r.client.Get(r.URL)
	if err != nil {
		reporter.add(trace.Errorf("healthz check failed: %v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		reporter.add(trace.Errorf("unexpected HTTP status: %s", http.StatusText(resp.StatusCode)))
		return
	}
	err = r.checkerFunc(resp.Body)
	if err != nil {
		reporter.add(err)
	} else {
		reporter.addProbe(&pb.Probe{
			Status: pb.Probe_Running,
		})
	}
}

func newHTTPHealthzChecker(URL string, checkerFunc checkerFunc) checker {
	client := &http.Client{
		Timeout: healthzCheckTimeout,
	}
	return &httpHealthzChecker{
		URL:         URL,
		client:      client,
		checkerFunc: checkerFunc,
	}
}

func newUnixSocketHealthzChecker(URL, socketPath string, checkerFunc checkerFunc) checker {
	client := &http.Client{
		Timeout: healthzCheckTimeout,
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
	return &httpHealthzChecker{
		URL:         URL,
		client:      client,
		checkerFunc: checkerFunc,
	}
}
