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
	"bytes"
	"context"
	"io"
	"io/ioutil"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	kube "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const systemNamespace = "kube-system"

// kubeHealthz is httpResponseChecker that interprets health status of common kubernetes services.
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

// KubeStatusChecker is a function that can check status of kubernetes services.
type KubeStatusChecker func(ctx context.Context, client *kube.Clientset) error

// KubeChecker implements Checker that can check and report problems
// with kubernetes services.
type KubeChecker struct {
	name       string
	masterURL  string
	configPath string
	checker    KubeStatusChecker
}

// ConnectToKube establishes a connection to kubernetes on the specified address
// and returns an API client.
func ConnectToKube(masterURL string, configPath string) (*kube.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags(masterURL, configPath)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	client, err := kube.NewForConfig(config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

// Name returns the name of this checker
func (r *KubeChecker) Name() string { return r.name }

// Check runs the wrapped kubernetes service checker function and reports
// status to the specified reporter
func (r *KubeChecker) Check(ctx context.Context, reporter health.Reporter) {
	client, err := r.connect()
	if err != nil {
		reporter.Add(NewProbeFromErr(r.name, "failed to connect to health endpoint", err))
		return
	}
	err = r.checker(ctx, client)
	if err != nil {
		reporter.Add(NewProbeFromErr(r.name, noErrorDetail, err))
		return
	}
	reporter.Add(&pb.Probe{
		Checker: r.name,
		Status:  pb.Probe_Running,
	})
}

func (r *KubeChecker) connect() (*kube.Clientset, error) {
	return ConnectToKube(r.masterURL, r.configPath)
}
