package monitoring

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/url"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	kube "k8s.io/kubernetes/pkg/client/unversioned"
)

const systemNamespace = "kube-system"

// kubeHealthz is checkerFunc that interprets health status of common kubernetes services.
func kubeHealthz(response io.Reader) error {
	payload, err := ioutil.ReadAll(response)
	if err != nil {
		return trace.Wrap(err)
	}
	if !bytes.Equal(payload, []byte("ok")) {
		return trace.Errorf("unexpected healthz response: %s", payload)
	}
	return nil
}

// KubeChecker is a Checker that communicates with the kube API server.
type KubeChecker func(client *kube.Client) error

// kubeChecker implements health checker that tests and reports problems
// with kubernetes services.
type kubeChecker struct {
	hostPort    string
	checkerFunc KubeChecker
}

// ConnectToKube establishes a connection to kubernetes on the specified address
// and returns an API client.
func ConnectToKube(hostPort string) (*kube.Client, error) {
	var baseURL *url.URL
	var err error
	if hostPort == "" {
		return nil, trace.Errorf("hostPort cannot be empty")
	}
	baseURL, err = url.Parse(hostPort)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	config := &kube.Config{
		Host: baseURL.String(),
	}
	client, err := kube.New(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

func (r *kubeChecker) check(reporter reporter) {
	client, err := r.connect()
	if err != nil {
		reporter.add(err)
		return
	}
	err = r.checkerFunc(client)
	if err != nil {
		reporter.add(err)
		return
	}
	reporter.addProbe(&pb.Probe{
		Status: pb.Probe_Running,
	})
}

func (r *kubeChecker) connect() (*kube.Client, error) {
	return ConnectToKube(r.hostPort)
}
