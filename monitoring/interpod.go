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
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	"github.com/blang/semver"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/typed/discovery"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/version"
)

// This file implements a functional pod communication test.
// It has been adopted from https://github.com/kubernetes/kubernetes/blob/master/test/e2e/networking.go

// testNamespace is the name of the namespace used for functional k8s tests.
const testNamespace = "planet-test"

// serviceNamePrefix is the prefix used to name test pods.
const serviceNamePrefix = "nettest-"

// interPodChecker is a Checker that runs a networking test in the cluster
// by scheduling pods and verifying communication.
type interPodChecker struct {
	*KubeChecker
	nettestContainerImage string
}

// NewInterPodChecker returns an instance of interPodChecker.
func NewInterPodChecker(kubeAddr, nettestContainerImage string) health.Checker {
	checker := &interPodChecker{
		nettestContainerImage: nettestContainerImage,
	}
	kubeChecker := &KubeChecker{
		name:     "networking",
		hostPort: kubeAddr,
		checker:  checker.testInterPodCommunication,
	}
	checker.KubeChecker = kubeChecker
	return kubeChecker
}

// testInterPodCommunication implements the inter-pod communication test.
func (r *interPodChecker) testInterPodCommunication(client *kube.Client) error {
	serviceName := generateName(serviceNamePrefix)
	if err := createNamespaceIfNeeded(client, testNamespace); err != nil {
		return trace.Wrap(err, "failed to create namespace `%v`", testNamespace)
	}

	const shouldWait = true
	const userName = "default"

	if _, err := getServiceAccount(client, testNamespace, userName, shouldWait); err != nil {
		return trace.Wrap(err, "service account has not yet been created - test postponed")
	}

	svc, err := client.Services(testNamespace).Create(&api.Service{
		ObjectMeta: api.ObjectMeta{
			Name: serviceName,
			Labels: map[string]string{
				"name": serviceName,
			},
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
			Selector: map[string]string{
				"name": serviceName,
			},
		},
	})
	if err != nil {
		return trace.Wrap(err, "failed to create test service named `%s`", serviceName)
	}

	cleanupService := func() {
		if err = client.Services(testNamespace).Delete(svc.Name); err != nil {
			log.Infof("failed to delete service %v: %v", svc.Name, err)
		}
	}
	defer cleanupService()

	listOptions := api.ListOptions{
		LabelSelector: labels.Everything(),
		FieldSelector: fields.Everything(),
	}
	nodes, err := client.Nodes().List(listOptions)
	if err != nil {
		return trace.Wrap(err, "failed to list nodes")
	}

	if len(nodes.Items) < 2 {
		return trace.Errorf("expected at least 2 ready nodes - got %d (%v)", len(nodes.Items), nodes.Items)
	}

	podNames, err := launchNetTestPodPerNode(client, nodes, serviceName, r.nettestContainerImage, testNamespace)
	if err != nil {
		return trace.Wrap(err, "failed to start `nettest` pod")
	}

	cleanupPods := func() {
		for _, podName := range podNames {
			if err = client.Pods(testNamespace).Delete(podName, nil); err != nil {
				log.Infof("failed to delete pod %s: %v", podName, err)
			}
		}
	}
	defer cleanupPods()

	for _, podName := range podNames {
		err = waitTimeoutForPodRunningInNamespace(client, podName, testNamespace, podStartTimeout)
		if err != nil {
			return trace.Wrap(err, "pod %s failed to transition to Running state", podName)
		}
	}

	passed := false

	getDetail := func(detail string) ([]byte, error) {
		proxyRequest, errProxy := getServicesProxyRequest(client, client.Get())
		if errProxy != nil {
			return nil, trace.Wrap(errProxy)
		}
		return proxyRequest.Namespace(testNamespace).
			Name(svc.Name).
			Suffix(detail).
			DoRaw()
	}

	getDetails := func() ([]byte, error) { return getDetail("read") }
	getStatus := func() ([]byte, error) { return getDetail("status") }

	var body []byte
	timeout := time.Now().Add(2 * time.Minute)
	for i := 0; !passed && timeout.After(time.Now()); i++ {
		time.Sleep(2 * time.Second)
		body, err = getStatus()
		if err != nil {
			log.Infof("attempt %v: service/pod still starting: %v)", i, err)
			continue
		}
		// validate if the container was able to find peers
		switch {
		case string(body) == "pass":
			passed = true
		case string(body) == "running":
			log.Infof("attempt %v: test still running", i)
		case string(body) == "fail":
			if body, err = getDetails(); err != nil {
				return trace.Wrap(err, "failed to read test details")
			} else {
				return trace.Wrap(err, "containers failed to find peers")
			}
		case strings.Contains(string(body), "no endpoints available"):
			log.Infof("attempt %v: waiting on service/endpoints", i)
		default:
			return trace.Errorf("unexpected response: [%s]", body)
		}
	}
	return nil
}

// podStartTimeout defines the amount of time to wait for a pod to start.
const podStartTimeout = 15 * time.Second

// pollInterval defines the amount of time to wait between attempts to poll pods/nodes.
const pollInterval = 2 * time.Second

// podCondition is an interface to verify the specific pod condition.
type podCondition func(pod *api.Pod) (bool, error)

// waitTimeoutForPodRunningInNamespace waits for a pod in the specified namespace
// to transition to 'Running' state within the specified amount of time.
func waitTimeoutForPodRunningInNamespace(client *kube.Client, podName string, namespace string, timeout time.Duration) error {
	return waitForPodCondition(client, namespace, podName, "running", timeout, func(pod *api.Pod) (bool, error) {
		if pod.Status.Phase == api.PodRunning {
			log.Infof("found pod '%s' on node '%s'", podName, pod.Spec.NodeName)
			return true, nil
		}
		if pod.Status.Phase == api.PodFailed {
			return true, trace.Errorf("pod in failed status: %s", fmt.Sprintf("%#v", pod))
		}
		return false, nil
	})
}

// waitForPodCondition waits until a pod is in the given condition within the specified amount of time.
func waitForPodCondition(client *kube.Client, ns, podName, desc string, timeout time.Duration, condition podCondition) error {
	log.Infof("waiting up to %v for pod %s status to be %s", timeout, podName, desc)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(pollInterval) {
		pod, err := client.Pods(ns).Get(podName)
		if err != nil {
			log.Infof("get pod %s in namespace '%s' failed, ignoring for %v: %v",
				podName, ns, pollInterval, err)
			continue
		}
		done, err := condition(pod)
		if done {
			// TODO: update to latest trace to wrap nil
			if err != nil {
				return trace.Wrap(err)
			}
			log.Infof("waiting for pod succeeded")
			return nil
		}
		log.Infof("waiting for pod %s in namespace '%s' status to be '%s'"+
			"(found phase: %q, readiness: %t) (%v elapsed)",
			podName, ns, desc, pod.Status.Phase, podReady(pod), time.Since(start))
	}
	return trace.Errorf("gave up waiting for pod '%s' to be '%s' after %v", podName, desc, timeout)
}

// launchNetTestPodPerNode schedules a new test pod on each of specified nodes
// using the specified containerImage.
func launchNetTestPodPerNode(client *kube.Client, nodes *api.NodeList, name, containerImage, namespace string) ([]string, error) {
	podNames := []string{}
	totalPods := len(nodes.Items)

	for _, node := range nodes.Items {
		pod, err := client.Pods(namespace).Create(&api.Pod{
			ObjectMeta: api.ObjectMeta{
				GenerateName: name + "-",
				Labels: map[string]string{
					"name": name,
				},
			},
			Spec: api.PodSpec{
				Containers: []api.Container{
					{
						Name:  "webserver",
						Image: containerImage,
						Args: []string{
							"-service=" + name,
							// `nettest` container finds peers by looking up list of service endpoints
							fmt.Sprintf("-peers=%d", totalPods),
							"-namespace=" + namespace},
						Ports: []api.ContainerPort{{ContainerPort: 8080}},
					},
				},
				NodeName:      node.Name,
				RestartPolicy: api.RestartPolicyNever,
			},
		})
		if err != nil {
			return nil, trace.Wrap(err, "failed to create pod")
		}
		log.Infof("created pod %s on node %s", pod.ObjectMeta.Name, node.Name)
		podNames = append(podNames, pod.ObjectMeta.Name)
	}
	return podNames, nil
}

// podReady returns whether pod has a condition of `Ready` with a status of true.
func podReady(pod *api.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == api.PodReady && cond.Status == api.ConditionTrue {
			return true
		}
	}
	return false
}

// createNamespaceIfNeeded creates a namespace if not already created.
func createNamespaceIfNeeded(client *kube.Client, namespace string) error {
	log.Infof("creating %s namespace", namespace)
	if _, err := client.Namespaces().Get(namespace); err != nil {
		log.Infof("%s namespace not found: %v", namespace, err)
		_, err = client.Namespaces().Create(&api.Namespace{ObjectMeta: api.ObjectMeta{Name: namespace}})
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// generateName generates a name for a kubernetes object.
// The name generated is guaranteed to satisfy kubernetes requirements.
func generateName(prefix string) string {
	return api.SimpleNameGenerator.GenerateName(prefix)
}

// getServiceAccount retrieves the service account with the specified name
// in the provided namespace.
func getServiceAccount(c *kube.Client, ns, name string, shouldWait bool) (*api.ServiceAccount, error) {
	if !shouldWait {
		return c.ServiceAccounts(ns).Get(name)
	}

	const interval = time.Second
	const timeout = 10 * time.Second

	var err error
	var user *api.ServiceAccount
	if err = wait.Poll(interval, timeout, func() (bool, error) {
		user, err = c.ServiceAccounts(ns).Get(name)
		if errors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}); err != nil {
		return nil, trace.Wrap(err)
	}
	return user, nil
}

var subResourceServiceAndNodeProxyVersion = version.MustParse("v1.2.0")

// getServicesProxyRequest returns the service request based on the server version
func getServicesProxyRequest(c *kube.Client, request *restclient.Request) (*restclient.Request, error) {
	subResourceProxyAvailable, err := serverVersionGTE(subResourceServiceAndNodeProxyVersion, c)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if subResourceProxyAvailable {
		return request.Resource("services").SubResource("proxy"), nil
	}
	return request.Prefix("proxy").Resource("services"), nil
}

// TODO: this functionality eventually becomes part of client.VersionInterface
// serverVersionGTE determines if server version >= v
func serverVersionGTE(v semver.Version, c discovery.ServerVersionInterface) (bool, error) {
	serverVersion, err := c.ServerVersion()
	if err != nil {
		return false, trace.Wrap(err, "unable to get server version")
	}
	parsedVersion, err := version.Parse(serverVersion.GitVersion)
	if err != nil {
		return false, trace.Wrap(err, "unable to parse server version %q", serverVersion.GitVersion)
	}
	return parsedVersion.GTE(v), nil
}
