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

package agent

import (
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gravitational/satellite/agent/cache"
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"
	"github.com/gravitational/satellite/lib/history/sqlite"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

// Agent is the interface to interact with the monitoring agent.
type Agent interface {
	// Start starts agent's background jobs.
	Start() error
	// Close stops background activity and releases resources.
	Close() error
	// Join makes an attempt to join a cluster specified by the list of peers.
	Join(peers []string) error
	// LocalStatus reports the health status of the local agent node.
	LocalStatus() *pb.NodeStatus
	// IsMember returns whether this agent is already a member of serf cluster
	IsMember() bool
	// GetConfig returns the agent configuration.
	GetConfig() Config
	// CheckerRepository allows to add checks to the agent.
	health.CheckerRepository
}

// Config defines satellite configuration.
type Config struct {
	// Name of the agent unique within the cluster.
	// Names are used as a unique id within a serf cluster, so
	// it is important to avoid clashes.
	//
	// Name must match the name of the local serf agent so that the agent
	// can match itself to a serf member.
	Name string

	// NodeName is the name assigned by Kubernetes to this node
	NodeName string

	// RPCAddrs is a list of addresses agent binds to for RPC traffic.
	//
	// Usually, at least two address are used for operation.
	// Localhost is a convenience for local communication.  Cluster-visible
	// IP is required for proper inter-communication between agents.
	RPCAddrs []string

	// CAFile specifies the path to TLS Certificate Authority file
	CAFile string

	// CertFile specifies the path to TLS certificate file
	CertFile string

	// KeyFile specifies the path to TLS certificate key file
	KeyFile string

	// RPC address of local serf node.
	SerfRPCAddr string

	// Address to listen on for web interface and telemetry for Prometheus metrics.
	MetricsAddr string

	// Peers lists the nodes that are part of the initial serf cluster configuration.
	// This is not a final cluster configuration and new nodes or node updates
	// are still possible.
	Peers []string

	// Set of tags for the agent.
	// Tags is a trivial means for adding extra semantic information to an agent.
	Tags map[string]string

	// DBPath specifies the location of the database to be used to store timeline.
	DBPath string

	// RetentionDuration specifies the duration to store timeline events.
	RetentionDuration time.Duration

	// Cache is a short-lived storage used by the agent to persist latest health stats.
	cache.Cache
}

// New creates an instance of an agent based on configuration options given in config.
func New(config *Config) (Agent, error) {
	if err := config.Check(); err != nil {
		return nil, trace.Wrap(err)
	}

	clientConfig := serf.Config{
		Addr: config.SerfRPCAddr,
	}
	client, err := NewSerfClient(clientConfig)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to serf")
	}

	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}

	err = client.UpdateTags(config.Tags, nil)
	if err != nil {
		return nil, trace.Wrap(err, "failed to update serf agent tags")
	}

	metricsListener, err := net.Listen("tcp", config.MetricsAddr)
	if err != nil {
		return nil, trace.Wrap(err, "failed to serve prometheus metrics")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timelineInitTimeout)
	defer cancel()

	timeline, err := sqlite.NewTimeline(ctx, sqlite.Config{
		DBPath:            config.DBPath,
		RetentionDuration: config.RetentionDuration,
		Clock:             clockwork.NewRealClock(),
	})
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize timeline")
	}

	clock := clockwork.NewRealClock()
	agent := &agent{
		Config:                  *config,
		SerfClient:              client,
		Timeline:                timeline,
		name:                    config.Name,
		cache:                   config.Cache,
		dialRPC:                 DefaultDialRPC(config.CAFile, config.CertFile, config.KeyFile),
		statusClock:             clock,
		recycleClock:            clock,
		timelineClock:           clock,
		statusQueryReplyTimeout: statusQueryReplyTimeout,
		localStatus:             emptyNodeStatus(config.Name),
		metricsListener:         metricsListener,
		done:                    make(chan struct{}),
	}

	agent.rpc, err = newRPCServer(agent, config.CAFile, config.CertFile, config.KeyFile, config.RPCAddrs)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create RPC server")
	}
	return agent, nil
}

// Check validates this configuration object
func (r Config) Check() error {
	var errors []error

	if r.CAFile == "" {
		errors = append(errors, trace.BadParameter("certificate authority file must be provided"))
	}

	if r.CertFile == "" {
		errors = append(errors, trace.BadParameter("certificate must be provided"))
	}

	if r.KeyFile == "" {
		errors = append(errors, trace.BadParameter("certificate key must be provided"))
	}

	if r.Name == "" {
		errors = append(errors, trace.BadParameter("agent name cannot be empty"))
	}

	if len(r.RPCAddrs) == 0 {
		errors = append(errors, trace.BadParameter("at least one RPC address must be provided"))
	}

	return trace.NewAggregate(errors...)
}

type agent struct {
	health.Checkers

	metricsListener net.Listener

	// SerfClient provides access to the serf agent.
	SerfClient SerfClient

	// Name of this agent.  Must be the same as the serf agent's name
	// running on the same node.
	name string

	// RPC server used by agent for client communication as well as
	// status sync with other agents.
	rpc RPCServer

	// cache persists node status history.
	cache cache.Cache

	// dialRPC is a factory function to create clients to other agents.
	// If future, agent address discovery will happen through serf.
	dialRPC DialRPC

	// done is a channel used for cleanup.
	done chan struct{}

	// TODO: do we need separate clocks here?
	// These clocks abstract away access to the time package to allow
	// testing.
	statusClock   clockwork.Clock
	recycleClock  clockwork.Clock
	timelineClock clockwork.Clock

	mu sync.Mutex
	// localStatus is the last obtained local node status.
	localStatus *pb.NodeStatus

	// statusQueryReplyTimeout specifies the maximum amount of time to wait for status reply
	// from remote nodes during status collection.
	// Defaults to statusQueryReplyTimeout if unspecified
	statusQueryReplyTimeout time.Duration

	// Config is the agent configuration.
	Config

	// Timeline keeps track of status change events.
	Timeline history.Timeline
}

// DialRPC returns RPC client for the provided Serf member.
type DialRPC func(context.Context, *serf.Member) (*client, error)

// GetConfig returns the agent configuration.
func (r *agent) GetConfig() Config {
	return r.Config
}

// Start starts the agent's background tasks.
func (r *agent) Start() error {
	errChan := make(chan error, 1)

	go r.statusUpdateLoop()
	go r.timelineUpdateLoop(context.TODO())

	if r.metricsListener != nil {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			if err := http.Serve(r.metricsListener, nil); err != nil {
				errChan <- trace.Wrap(err)
			}
		}()
	}

	select {
	case err := <-errChan:
		return err
	case <-time.After(1 * time.Second):
		return nil
	}
}

// IsMember returns true if this agent is a member of the serf cluster
func (r *agent) IsMember() bool {
	members, err := r.SerfClient.Members()
	if err != nil {
		log.Errorf("failed to retrieve members: %v", trace.DebugReport(err))
		return false
	}
	// if we're the only one, consider that we're not in the cluster yet
	// (cause more often than not there are more than 1 member)
	if len(members) == 1 && members[0].Name == r.name {
		return false
	}
	for _, member := range members {
		if member.Name == r.name {
			return true
		}
	}
	return false
}

// Join attempts to join a serf cluster identified by peers.
func (r *agent) Join(peers []string) error {
	noReplay := false
	numJoined, err := r.SerfClient.Join(peers, noReplay)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("joined %d nodes", numJoined)
	return nil
}

// Close stops all background activity and releases the agent's resources.
func (r *agent) Close() (err error) {
	var errors []error
	if r.metricsListener != nil {
		err = r.metricsListener.Close()
		if err != nil {
			errors = append(errors, trace.Wrap(err))
		}
	}

	r.rpc.Stop()
	close(r.done)

	err = r.SerfClient.Close()
	if err != nil {
		errors = append(errors, trace.Wrap(err))
	}

	if len(errors) > 0 {
		return trace.NewAggregate(errors...)
	}
	return nil
}

// LocalStatus reports the status of the local agent node.
func (r *agent) LocalStatus() *pb.NodeStatus {
	return r.recentLocalStatus()
}

// runChecks executes the monitoring tests configured for this agent in parallel.
func (r *agent) runChecks(ctx context.Context) *pb.NodeStatus {
	// semaphoreCh limits the number of concurrent checkers
	semaphoreCh := make(chan struct{}, maxConcurrentCheckers)
	// channel for collecting resulting health probes
	probeCh := make(chan health.Probes, len(r.Checkers))

	ctxChecks, cancelChecks := context.WithTimeout(ctx, checksTimeout)
	defer cancelChecks()

	for _, c := range r.Checkers {
		select {
		case semaphoreCh <- struct{}{}:
			go runChecker(ctxChecks, c, probeCh, semaphoreCh)
		case <-ctx.Done():
			log.Warnf("Timed out running tests: %v.", ctx.Err())
			return emptyNodeStatus(r.name)
		}
	}

	var probes health.Probes
	for i := 0; i < len(r.Checkers); i++ {
		select {
		case probe := <-probeCh:
			probes = append(probes, probe...)
		case <-ctx.Done():
			log.Warnf("Timed out collecting test results: %v.", ctx.Err())
			return &pb.NodeStatus{
				Name:   r.name,
				Status: pb.NodeStatus_Degraded,
				Probes: probes.GetProbes(),
			}
		}
	}

	return &pb.NodeStatus{
		Name:   r.name,
		Status: probes.Status(),
		Probes: probes.GetProbes(),
	}
}

// runChecker executes the specified checker and reports results on probeCh.
// If the checker panics, the resulting probe will describe the checker failure.
// Semaphore channel is guaranteed to receive a value upon completion.
func runChecker(ctx context.Context, checker health.Checker, probeCh chan<- health.Probes, semaphoreCh <-chan struct{}) {
	defer func() {
		if err := recover(); err != nil {
			var probes health.Probes
			probes.Add(&pb.Probe{
				Checker:  checker.Name(),
				Status:   pb.Probe_Failed,
				Severity: pb.Probe_Critical,
				Error:    trace.Errorf("checker panicked: %v\n%s", err, debug.Stack()).Error(),
			})
			probeCh <- probes
		}
		// release checker slot
		<-semaphoreCh
	}()

	log.Debugf("Running checker %q.", checker.Name())

	ctxProbe, cancelProbe := context.WithTimeout(ctx, probeTimeout)
	defer cancelProbe()

	checkCh := make(chan health.Probes, 1)
	go func() {
		var probes health.Probes
		checker.Check(ctxProbe, &probes)
		checkCh <- probes
	}()

	select {
	case probes := <-checkCh:
		probeCh <- probes
	case <-ctx.Done():
		var probes health.Probes
		probes.Add(&pb.Probe{
			Checker:  checker.Name(),
			Status:   pb.Probe_Failed,
			Severity: pb.Probe_Critical,
			Error:    "checker does not comply with specified context, potential goroutine leak",
		})
		probeCh <- probes
	}
}

// statusUpdateLoop is a long running background process that periodically
// updates the health status of the cluster by querying status of other active
// cluster members.
func (r *agent) statusUpdateLoop() {
	statusUpdateCh := r.statusClock.After(statusUpdateTimeout)
	recycleCh := r.recycleClock.After(recycleTimeout)

	// collectCh is a channel that receives updates when the status collecting process
	// has finished collecting
	collectCh := make(chan struct{})
	for {
		select {
		case <-statusUpdateCh:
			ctx, cancel := context.WithTimeout(context.Background(), r.statusQueryReplyTimeout)
			go func() {
				defer cancel() // close context if collection finishes before the deadline
				status, err := r.collectStatus(ctx)
				if err != nil {
					log.Warnf("Error collecting system status: %v.", err)
				}
				if status != nil {
					if err = r.cache.UpdateStatus(status); err != nil {
						log.Warnf("Error updating system status in cache: %v", err)
					}
				}
				select {
				case <-r.done:
				case collectCh <- struct{}{}:
				}
			}()
			select {
			case <-collectCh:
				statusUpdateCh = r.statusClock.After(statusUpdateTimeout)
			case <-r.done:
				cancel()
				return
			}
		case <-recycleCh:
			err := r.cache.Recycle()
			if err != nil {
				log.Warningf("error recycling stats: %v", err)
			}
			recycleCh = r.recycleClock.After(recycleTimeout)
		case <-r.done:
			return
		}
	}
}

// collectStatus obtains the cluster status by querying statuses of
// known cluster members.
func (r *agent) collectStatus(ctx context.Context) (systemStatus *pb.SystemStatus, err error) {
	systemStatus = &pb.SystemStatus{
		Status:    pb.SystemStatus_Unknown,
		Timestamp: pb.NewTimeToProto(r.statusClock.Now()),
	}

	members, err := r.SerfClient.Members()
	if err != nil {
		log.WithError(err).Warn("Failed to query serf members.")
		return nil, trace.Wrap(err, "failed to query serf members")
	}
	members = filterLeft(members)

	log.Debugf("Started collecting statuses from members %v.", members)

	ctxNode, cancelNode := context.WithTimeout(ctx, nodeStatusTimeout)
	defer cancelNode()

	statusCh := make(chan *statusResponse, len(members))
	for _, member := range members {
		if r.name == member.Name {
			go r.getLocalStatus(ctxNode, member, statusCh)
		} else {
			go r.getStatusFrom(ctxNode, member, statusCh)
		}
	}

L:
	for i := 0; i < len(members); i++ {
		select {
		case status := <-statusCh:
			log.Debugf("Retrieved status from %v: %v.", status.member, status.NodeStatus)
			nodeStatus := status.NodeStatus
			if status.err != nil {
				log.Warnf("Failed to query node %s(%v) status: %v.",
					status.member.Name, status.member.Addr, status.err)
				nodeStatus = unknownNodeStatus(&status.member)
			}
			systemStatus.Nodes = append(systemStatus.Nodes, nodeStatus)
		case <-ctx.Done():
			log.Warnf("Timed out collecting node statuses: %v.", ctx.Err())
			// With insufficient status responses received, system status
			// will be automatically degraded
			break L
		}
	}
	setSystemStatus(systemStatus, members)

	return systemStatus, nil
}

// collectLocalStatus executes monitoring tests on the local node.
func (r *agent) collectLocalStatus(ctx context.Context, local *serf.Member) (status *pb.NodeStatus) {
	status = r.runChecks(ctx)
	status.MemberStatus = statusFromMember(local)
	r.mu.Lock()
	r.localStatus = status
	r.mu.Unlock()

	return status
}

// getLocalStatus obtains local node status.
func (r *agent) getLocalStatus(ctx context.Context, local serf.Member, respc chan<- *statusResponse) {
	status := r.collectLocalStatus(ctx, &local)
	resp := &statusResponse{
		NodeStatus: status,
		member:     local,
	}
	select {
	case respc <- resp:
	case <-r.done:
	}
}

// getStatusFrom obtains node status from the node identified by member.
func (r *agent) getStatusFrom(ctx context.Context, member serf.Member, respc chan<- *statusResponse) {
	client, err := r.dialRPC(ctx, &member)
	resp := &statusResponse{member: member}
	if err != nil {
		resp.err = trace.Wrap(err)
	} else {
		defer client.Close()
		var status *pb.NodeStatus
		status, err = client.LocalStatus(ctx)
		if err != nil {
			resp.err = trace.Wrap(err)
		} else {
			resp.NodeStatus = status
		}
	}
	select {
	case respc <- resp:
	case <-r.done:
	}
}

// timelineUpdateLoop periodically updates the status timeline.
func (r *agent) timelineUpdateLoop(ctx context.Context) {
	ticker := r.timelineClock.NewTicker(statusUpdateTimeout)
	defer ticker.Stop()

	// index is used select another member of the cluster.
	// Type uint16 so limited to 0 - 65535. Assuming clusters will not have
	// more than that many members.
	var index uint16

	for range ticker.Chan() {
		if utils.IsContextDone(ctx) {
			log.Info("Timeline update loop is stopping.")
			return
		}

		if err := r.updateLocalChanges(ctx); err != nil {
			log.WithError(err).Warn("Error updating local timeline changes.")
			continue
		}

		member, err := r.selectMember(int(index))
		if err != nil {
			log.WithError(err).Warn("Failed to select cluster member.")
			continue
		}

		if err := r.updateRemoteChanges(ctx, member); err != nil {
			log.WithError(err).Warn("Error updating local timeline changes.")
			continue
		}

		index++
	}
}

// updateLocalChanges updates the timeline with any new status changes found on
// the local node.
func (r *agent) updateLocalChanges(ctx context.Context) error {
	ctxUpdate, cancel := context.WithTimeout(ctx, updateTimelineTimeout)
	defer cancel()

	if err := r.Timeline.RecordStatus(ctxUpdate, r.LocalStatus()); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// updateRemoteChanges updates the timeline with events from another member in
// the cluster.
func (r *agent) updateRemoteChanges(ctx context.Context, member serf.Member) error {
	ctxQuery, cancel := context.WithTimeout(ctx, r.statusQueryReplyTimeout)
	defer cancel()
	timeline, err := r.getTimelineFrom(ctxQuery, member)
	if err != nil {
		return trace.Wrap(err)
	}

	ctxUpdate, cancel := context.WithTimeout(ctx, updateTimelineTimeout)
	defer cancel()
	if err := r.Timeline.RecordTimeline(ctxUpdate, timeline); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// selectMember will select a member based using the index.
// The index is unit16 so will be limited to ~65000. Assuming clusters will not
// have over that many members.
func (r *agent) selectMember(index int) (member serf.Member, err error) {
	members, err := r.SerfClient.Members()
	if err != nil {
		return member, trace.Wrap(err, "failed to query serf members")
	}
	// Ignore members that have left the cluster.
	members = filterLeft(members)
	// Ignore self.
	members = filterMember(members, r.name)

	numAvailable := len(members)
	return members[index%numAvailable], nil
}

// getTimelineFrom obtains timeline from the node identified by member.
func (r *agent) getTimelineFrom(ctx context.Context, member serf.Member) (events []*pb.TimelineEvent, err error) {
	client, err := r.dialRPC(ctx, &member)
	if err != nil {
		return events, trace.Wrap(err)
	}
	defer client.Close()

	timeline, err := client.Timeline(ctx, &pb.TimelineRequest{})
	if err != nil {
		return events, trace.Wrap(err)
	}
	return timeline.GetEvents(), nil
}

// recentStatus returns the last known cluster status.
func (r *agent) recentStatus() (status *pb.SystemStatus, err error) {
	status, err = r.cache.RecentStatus()
	if err == nil && status == nil {
		status = pb.EmptyStatus()
	}
	return status, trace.Wrap(err)
}

// recentLocalStatus returns the last known local node status.
func (r *agent) recentLocalStatus() *pb.NodeStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.localStatus
}

// TODO: Optimization - use a different data structure for storing serf members
// if we need to constantly filter the list.

// filterLeft filters out members that have left the serf cluster
func filterLeft(members []serf.Member) (result []serf.Member) {
	result = make([]serf.Member, 0, len(members))
	for _, member := range members {
		if member.Status == MemberLeft {
			// Skip
			continue
		}
		result = append(result, member)
	}
	return result
}

// filterMember filters out member by name.
func filterMember(members []serf.Member, name string) (result []serf.Member) {
	for i, member := range members {
		if member.Name == name {
			return append(members[:i], members[i+1:]...)
		}
	}
	return result
}

// statusResponse describes a status response from a background process that obtains
// health status on the specified serf node.
type statusResponse struct {
	*pb.NodeStatus
	member serf.Member
	err    error
}
