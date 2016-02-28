package monitoring

import (
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/wait"
)

// This file implements a functional pod communication test.
// It has been adopted from https://github.com/kubernetes/kubernetes/blob/master/test/e2e/networking.go

// testNamespace is the name of the namespace used for functional k8s tests.
const testNamespace = "planet-test"

// serviceNamePrefix is the prefix used to name test pods.
const serviceNamePrefix = "nettest-"

// intraPodChecker is a Checker that runs a networking test in the cluster
// by scheduling pods and verifying communication.
type intraPodChecker struct {
	*KubeChecker
	nettestContainerImage string
}

// NewIntraPodChecker returns an instance of intraPodChecker.
func NewIntraPodChecker(kubeAddr, nettestContainerImage string) health.Checker {
	checker := &intraPodChecker{
		nettestContainerImage: nettestContainerImage,
	}
	kubeChecker := &KubeChecker{
		name:     "networking",
		hostPort: kubeAddr,
		checker:  checker.testIntraPodCommunication,
	}
	checker.KubeChecker = kubeChecker
	return kubeChecker
}

// testIntraPodCommunication implements the intra-pod communication test.
func (r *intraPodChecker) testIntraPodCommunication(client *kube.Client) error {
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
				TargetPort: util.NewIntOrStringFromInt(8080),
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

	nodes, err := client.Nodes().List(labels.Everything(), fields.Everything())
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

	var body []byte
	getDetails := func() ([]byte, error) {
		return client.Get().
			Namespace(testNamespace).
			Prefix("proxy").
			Resource("services").
			Name(svc.Name).
			Suffix("read").
			DoRaw()
	}

	getStatus := func() ([]byte, error) {
		return client.Get().
			Namespace(testNamespace).
			Prefix("proxy").
			Resource("services").
			Name(svc.Name).
			Suffix("status").
			DoRaw()
	}

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
