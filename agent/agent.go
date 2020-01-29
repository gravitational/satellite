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
	"github.com/gravitational/satellite/lib/client"
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
	// LastSeen returns the last seen timestamp from the specified member.
	LastSeen(name string) time.Time
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

	// SerfConfig specifies serf client configuration.
	SerfConfig serf.Config

	// Address to listen on for web interface and telemetry for Prometheus metrics.
	MetricsAddr string

	// Peers lists the nodes that are part of the initial serf cluster configuration.
	// This is not a final cluster configuration and new nodes or node updates
	// are still possible.
	Peers []string

	// Set of tags for the agent.
	// Tags is a trivial means for adding extra semantic information to an agent.
	Tags map[string]string

	// TimelineConfig specifies sqlite timeline configuration.
	TimelineConfig sqlite.Config

	// LocalTimelineConfig specifies local timeline configuration.
	LocalTimelineConfig sqlite.Config

	// Clock to be used for internal time keeping.
	Clock clockwork.Clock

	// Cache is a short-lived storage used by the agent to persist latest health stats.
	cache.Cache
}

// CheckAndSetDefaults validates this configuration object.
// Config values that were not specified will be set to their default values if
// available.
func (r *Config) CheckAndSetDefaults() error {
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
	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	if r.Clock == nil {
		r.Clock = clockwork.NewRealClock()
	}
	return trace.NewAggregate(errors...)
}

type agent struct {
	sync.Mutex
	health.Checkers

	metricsListener net.Listener

	// RPC server used by agent for client communication as well as
	// status sync with other agents.
	rpc RPCServer

	// dialRPC is a factory function to create clients to other agents.
	// If future, agent address discovery will happen through serf.
	dialRPC client.DialRPC

	// done is a channel used for cleanup.
	done chan struct{}

	// localStatus is the last obtained local node status.
	localStatus *pb.NodeStatus

	// statusQueryReplyTimeout specifies the maximum amount of time to wait for status reply
	// from remote nodes during status collection.
	// Defaults to statusQueryReplyTimeout if unspecified
	statusQueryReplyTimeout time.Duration

	// lastSeen keeps track of the last seen timestamp from a cluster member.
	// The last seen timestamp can be queried by a member and be used to
	// filter out events that have already been recorded by this member.
	lastSeen map[string]time.Time

	// Config is the agent configuration.
	Config

	// SerfClient provides access to the serf agent.
	SerfClient SerfClient

	// Timeline keeps track of all timeline events in the cluster. This timeline
	// is only used on members that have the role 'master'.
	Timeline history.Timeline

	// LocalTimeline keeps track of local timeline events.
	LocalTimeline history.Timeline
}

// New creates an instance of an agent based on configuration options given in config.
func New(config *Config) (Agent, error) {
	if err := config.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	serfClient, err := initSerfClient(config.SerfConfig, config.Tags)
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize serf client")
	}

	// TODO: do we need to initialize metrics listener in constructor?
	// Move to Start?
	metricsListener, err := net.Listen("tcp", config.MetricsAddr)
	if err != nil {
		return nil, trace.Wrap(err, "failed to serve prometheus metrics")
	}

	timeline, err := initTimeline(config.TimelineConfig)
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize timeline")
	}

	localTimeline, err := initTimeline(config.LocalTimelineConfig)
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize local timeline")
	}

	agent := &agent{
		dialRPC:                 client.DefaultDialRPC(config.CAFile, config.CertFile, config.KeyFile),
		statusQueryReplyTimeout: statusQueryReplyTimeout,
		localStatus:             emptyNodeStatus(config.Name),
		metricsListener:         metricsListener,
		lastSeen:                make(map[string]time.Time),
		done:                    make(chan struct{}),
		Config:                  *config,
		SerfClient:              serfClient,
		Timeline:                timeline,
		LocalTimeline:           localTimeline,
	}

	agent.rpc, err = newRPCServer(agent, config.CAFile, config.CertFile, config.KeyFile, config.RPCAddrs)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create RPC server")
	}
	return agent, nil
}

// initSerfClient initializes a new serf client and modifies the client with
// the provided tags.
func initSerfClient(config serf.Config, tags map[string]string) (SerfClient, error) {
	client, err := NewSerfClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to serf")
	}
	if err = client.UpdateTags(tags, nil); err != nil {
		return nil, trace.Wrap(err, "failed to update serf agent tags")
	}
	return client, nil
}

// initTimeline initializes a new sqlite timeline.
func initTimeline(config sqlite.Config) (history.Timeline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timelineInitTimeout)
	defer cancel()
	return sqlite.NewTimeline(ctx, config)
}

// GetConfig returns the agent configuration.
func (r *agent) GetConfig() Config {
	return r.Config
}

// Start starts the agent's background tasks.
func (r *agent) Start() error {
	// TODO: Modify Start to accept a context.
	ctx, cancel := context.WithCancel(context.TODO())
	go r.recycleLoop(ctx)
	go r.statusUpdateLoop(ctx)
	go r.serveMetrics()

	go func() {
		<-r.done
		cancel()
	}()
	return nil
}

// serveMetrics registers the prometheus metrics handler and starts accepting
// connections on the metrics listener.
func (r *agent) serveMetrics() {
	if r.metricsListener == nil {
		return
	}

	http.Handle("/metrics", promhttp.Handler())
	err := http.Serve(r.metricsListener, nil)
	if err == http.ErrServerClosed {
		log.WithError(err).Debug("Metrics listener has been shutdown/closed.")
	}
	if err != nil {
		log.WithError(err).Errorf("Failed to server metrics.")
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
	if len(members) == 1 && members[0].Name == r.Name {
		return false
	}
	for _, member := range members {
		if member.Name == r.Name {
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

// LastSeen returns the last seen timestamp from the specified member.
func (r *agent) LastSeen(name string) time.Time {
	r.Lock()
	defer r.Unlock()

	if timestamp, ok := r.lastSeen[name]; ok {
		return timestamp
	}

	// If a last seen timestamp is not recorded for this member, initialize
	// timestamp for this member.
	r.lastSeen[name] = time.Time{}
	return time.Time{}
}

// recordLastSeen records the last seen timestamp for the specified member.
func (r *agent) recordLastSeen(name string, lastSeen time.Time) {
	r.Lock()
	defer r.Unlock()
	r.lastSeen[name] = lastSeen
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
			return emptyNodeStatus(r.Name)
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
				Name:   r.Name,
				Status: pb.NodeStatus_Degraded,
				Probes: probes.GetProbes(),
			}
		}
	}

	return &pb.NodeStatus{
		Name:   r.Name,
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

// recycleLoop periodically recycles the cache.
func (r *agent) recycleLoop(ctx context.Context) {
	ticker := r.Clock.NewTicker(recycleTimeout)
	defer ticker.Stop()
	for range ticker.Chan() {
		if utils.IsContextDone(ctx) {
			log.Info("Recycle loop is stopping.")
			return
		}
		if err := r.Cache.Recycle(); err != nil {
			log.WithError(err).Warnf("Error recycling status.")
			continue
		}
	}
}

// statusUpdateLoop is a long running background process that periodically
// updates the health status of the cluster by querying status of other active
// cluster members.
func (r *agent) statusUpdateLoop(ctx context.Context) {
	ticker := r.Clock.NewTicker(statusUpdateTimeout)
	defer ticker.Stop()
	for range ticker.Chan() {
		if utils.IsContextDone(ctx) {
			log.Info("Status update loop is stopping.")
			return
		}
		if err := r.updateStatus(ctx); err != nil {
			log.WithError(err).Warnf("Failed to updates status.")
		}
	}
}

// updateStatus updates the current status.
func (r *agent) updateStatus(ctx context.Context) error {
	ctxStatus, cancel := context.WithTimeout(ctx, r.statusQueryReplyTimeout)
	defer cancel()
	status, err := r.collectStatus(ctxStatus)
	if err != nil {
		return trace.Wrap(err, "error collecting system status")
	}
	if status == nil {
		return nil
	}
	if err := r.Cache.UpdateStatus(status); err != nil {
		return trace.Wrap(err, "error updating system status in cache")
	}
	return nil
}

// collectStatus obtains the cluster status by querying statuses of
// known cluster members.
func (r *agent) collectStatus(ctx context.Context) (systemStatus *pb.SystemStatus, err error) {
	ctx, cancel := context.WithTimeout(ctx, statusUpdateTimeout)
	defer cancel()

	systemStatus = &pb.SystemStatus{
		Status:    pb.SystemStatus_Unknown,
		Timestamp: pb.NewTimeToProto(r.Clock.Now()),
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
		if r.Name == member.Name {
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
func (r *agent) collectLocalStatus(ctx context.Context, local *serf.Member) (status *pb.NodeStatus, err error) {
	status = r.runChecks(ctx)
	status.MemberStatus = statusFromMember(local)

	r.Lock()
	changes := history.DiffNode(r.Clock, r.localStatus, status)
	r.localStatus = status
	r.Unlock()

	/// TODO: handle recording of timeline outside of collection.
	if err := r.LocalTimeline.RecordTimeline(ctx, changes); err != nil {
		return status, trace.Wrap(err, "failed to record local timeline events")
	}

	if err := r.notifyMasters(ctx); err != nil {
		return status, trace.Wrap(err, "failed to notify master nodes of local timeline events")
	}

	return status, nil
}

// notifyMasters pushes new timeline events to all master nodes in the cluster.
func (r *agent) notifyMasters(ctx context.Context) error {
	members, err := r.SerfClient.Members()
	if err != nil {
		return trace.Wrap(err)
	}

	events, err := r.LocalTimeline.GetEvents(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	// TODO: async
	for _, member := range members {
		if role, ok := member.Tags["role"]; ok && Role(role) == RoleMaster {
			if err := r.notifyMaster(ctx, &member, events); err != nil {
				log.WithError(err).Warnf("Failed to notify %s of new timeline events.", member.Name)
			}
		}
	}

	return nil
}

// notifyMaster push new timeline events to the specified member.
func (r *agent) notifyMaster(ctx context.Context, member *serf.Member, events []*pb.TimelineEvent) error {
	client, err := r.dialRPC(ctx, member)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	// Filter events by last seen timestamp. Only push unrecorded events.
	resp, err := client.LastSeen(ctx, &pb.LastSeenRequest{Name: r.Name})
	if err != nil {
		return trace.Wrap(err)
	}

	// Subtract a second in case multiple events were recorded with the same timestamp.
	// If any events failed to push previously, they will be resent.
	filtered := filterByTimestamp(events, resp.GetTimestamp().ToTime().Add(-time.Second))

	for _, event := range filtered {
		if _, err := client.UpdateTimeline(ctx, &pb.UpdateRequest{Name: r.Name, Event: event}); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// getLocalStatus obtains local node status.
func (r *agent) getLocalStatus(ctx context.Context, local serf.Member, respc chan<- *statusResponse) {
	status, err := r.collectLocalStatus(ctx, &local)
	resp := &statusResponse{
		NodeStatus: status,
		member:     local,
		err:        err,
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

// recentStatus returns the last known cluster status.
func (r *agent) recentStatus() (status *pb.SystemStatus, err error) {
	status, err = r.Cache.RecentStatus()
	if err == nil && status == nil {
		status = pb.EmptyStatus()
	}
	return status, trace.Wrap(err)
}

// recentLocalStatus returns the last known local node status.
func (r *agent) recentLocalStatus() *pb.NodeStatus {
	r.Lock()
	defer r.Unlock()
	return r.localStatus
}

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

// filterByTimestamp filters out events that occurred before the provided
// timestamp.
func filterByTimestamp(events []*pb.TimelineEvent, timestamp time.Time) (filtered []*pb.TimelineEvent) {
	for _, event := range events {
		if event.GetTimestamp().ToTime().After(timestamp) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// statusResponse describes a status response from a background process that obtains
// health status on the specified serf node.
type statusResponse struct {
	*pb.NodeStatus
	member serf.Member
	err    error
}
