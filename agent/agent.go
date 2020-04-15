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
	"context"
	"fmt"
	"net"
	"net/http"
	"path"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gravitational/satellite/agent/cache"
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/lib/history"
	"github.com/gravitational/satellite/lib/history/sqlite"
	"github.com/gravitational/satellite/lib/membership"
	"github.com/gravitational/satellite/lib/rpc/client"

	"github.com/gravitational/trace"
	"github.com/gravitational/ttlmap"
	serf "github.com/hashicorp/serf/client"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

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

	// lastSeen keeps track of the last seen timestamp of an event from a
	// specific cluster member.
	// The last seen timestamp can be queried by a member and be used to
	// filter out events that have already been recorded by this agent.
	lastSeen *ttlmap.TTLMap

	// Config is the agent configuration.
	Config

	// ClusterMembership provides access to cluster membership service.
	ClusterMembership membership.ClusterMembership

	// ClusterTimeline keeps track of all timeline events in the cluster. This
	// timeline is only used by members that have the role 'master'.
	ClusterTimeline history.Timeline

	// LocalTimeline keeps track of local timeline events.
	LocalTimeline history.Timeline
}

// New creates an instance of an agent based on configuration options given in config.
func New(config *Config) (*agent, error) {
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

	localTimeline, err := initTimeline(config.TimelineConfig, "local.db")
	if err != nil {
		return nil, trace.Wrap(err, "failed to initialize local timeline")
	}

	// Only initialize cluster timeline for master nodes.
	var clusterTimeline history.Timeline
	var lastSeen *ttlmap.TTLMap
	if role, ok := config.Tags["role"]; ok && Role(role) == RoleMaster {
		clusterTimeline, err = initTimeline(config.TimelineConfig, "cluster.db")
		if err != nil {
			return nil, trace.Wrap(err, "failed to initialize timeline")
		}

		lastSeen, err = ttlmap.New(lastSeenCapacity)
		if err != nil {
			return nil, trace.Wrap(err, "failed to initialize last seen ttl map")
		}
	}

	agent := &agent{
		dialRPC:                 client.DefaultDialRPC(config.CAFile, config.CertFile, config.KeyFile),
		statusQueryReplyTimeout: statusQueryReplyTimeout,
		localStatus:             emptyNodeStatus(config.Name),
		metricsListener:         metricsListener,
		lastSeen:                lastSeen,
		done:                    make(chan struct{}),
		Config:                  *config,
		ClusterMembership:       serfClient,
		ClusterTimeline:         clusterTimeline,
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
func initSerfClient(config serf.Config, tags map[string]string) (*membership.RetryingClient, error) {
	client, err := membership.NewSerfClient(config)
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to serf")
	}
	if err = client.UpdateTags(tags, nil); err != nil {
		return nil, trace.Wrap(err, "failed to update serf agent tags")
	}
	return client, nil
}

// initTimeline initializes a new sqlite timeline. dbName specifies the
// SQLite database file name.
func initTimeline(config sqlite.Config, fileName string) (history.Timeline, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timelineInitTimeout)
	defer cancel()
	config.DBPath = path.Join(config.DBPath, fileName)
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
	members, err := r.ClusterMembership.Members()
	if err != nil {
		log.Errorf("failed to retrieve members: %v", trace.DebugReport(err))
		return false
	}
	// if we're the only one, consider that we're not in the cluster yet
	// (cause more often than not there are more than 1 member)
	if len(members) == 1 && members[0].Name() == r.Name {
		return false
	}
	for _, member := range members {
		if member.Name() == r.Name {
			return true
		}
	}
	return false
}

// Join attempts to join a serf cluster identified by peers.
func (r *agent) Join(peers []string) error {
	noReplay := false
	numJoined, err := r.ClusterMembership.Join(peers, noReplay)
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
	if r.done != nil {
		close(r.done)
	}

	err = r.ClusterMembership.Close()
	if err != nil {
		errors = append(errors, trace.Wrap(err))
	}

	if len(errors) > 0 {
		return trace.NewAggregate(errors...)
	}
	return nil
}

// Time reports the current server time.
func (r *agent) Time() time.Time {
	return r.Clock.Now()
}

// LocalStatus reports the status of the local agent node.
func (r *agent) LocalStatus() *pb.NodeStatus {
	return r.recentLocalStatus()
}

// LastSeen returns the last seen timestamp from the specified member.
// If no value is stored for the specific member, a timestamp will be
// initialized for the member with the zero value.
func (r *agent) LastSeen(name string) (lastSeen time.Time, err error) {
	if !hasRoleMaster(r.Tags) {
		return lastSeen, trace.BadParameter("requesting last seen timestamp from non master")
	}

	r.Lock()
	defer r.Unlock()

	if val, ok := r.lastSeen.Get(name); ok {
		if lastSeen, ok = val.(time.Time); !ok {
			return lastSeen, trace.BadParameter("got invalid type %T", val)
		}
	}

	// Reset ttl if successfully retrieved lastSeen.
	// Initialize value if lastSeen had not been previously stored.
	if err := r.lastSeen.Set(name, time.Time{}, lastSeenTTL); err != nil {
		return lastSeen, trace.Wrap(err, fmt.Sprintf("failed to initialize timestamp for %s", name))
	}

	return lastSeen, nil
}

// RecordLastSeen records the timestamp for the specified member.
// Attempts to record a last seen timestamp that is older than the currently
// recorded timestamp will be ignored.
func (r *agent) RecordLastSeen(name string, timestamp time.Time) error {
	if !hasRoleMaster(r.Tags) {
		return trace.BadParameter("attempting to record last seen timestamp for non master")
	}

	r.Lock()
	defer r.Unlock()

	var lastSeen time.Time
	if val, ok := r.lastSeen.Get(name); ok {
		if lastSeen, ok = val.(time.Time); !ok {
			return trace.BadParameter("got invalid type %T", val)
		}
	}

	// Ignore timestamp that is older than currently stored last seen timestamp.
	if timestamp.Before(lastSeen) {
		return nil
	}

	return r.lastSeen.Set(name, timestamp, lastSeenTTL)
}

// runChecks executes the monitoring tests configured for this agent in parallel.
func (r *agent) runChecks(ctx context.Context) *pb.NodeStatus {
	// semaphoreCh limits the number of concurrent checkers
	semaphoreCh := make(chan struct{}, maxConcurrentCheckers)
	// channel for collecting resulting health probes
	probeCh := make(chan health.Probes, len(r.Checkers))

	for _, c := range r.Checkers {
		select {
		case semaphoreCh <- struct{}{}:
			go runChecker(ctx, c, probeCh, semaphoreCh)
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

// GetTimeline returns the current cluster timeline.
func (r *agent) GetTimeline(ctx context.Context, params map[string]string) ([]*pb.TimelineEvent, error) {
	if hasRoleMaster(r.Tags) {
		return r.ClusterTimeline.GetEvents(ctx, params)
	}
	return nil, trace.BadParameter("requesting cluster timeline from non master")
}

// RecordClusterEvents records the events into the cluster timeline.
// Cluster timeline can only be updated if agent has role 'master'.
func (r *agent) RecordClusterEvents(ctx context.Context, events []*pb.TimelineEvent) error {
	if hasRoleMaster(r.Tags) {
		return r.ClusterTimeline.RecordEvents(ctx, events)
	}
	return trace.BadParameter("attempting to update cluster timeline of non master")
}

// RecordLocalEvents records the events into the local timeline.
func (r *agent) RecordLocalEvents(ctx context.Context, events []*pb.TimelineEvent) error {
	return r.LocalTimeline.RecordEvents(ctx, events)
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

	for {
		select {
		case <-ctx.Done():
			log.Info("Recycle loop is stopping.")
			return
		case <-ticker.Chan():
			if err := r.Cache.Recycle(); err != nil {
				log.WithError(err).Warn("Error recycling status.")
			}
		}
	}
}

// statusUpdateLoop is a long running background process that periodically
// updates the health status of the cluster by querying status of other active
// cluster members.
func (r *agent) statusUpdateLoop(ctx context.Context) {
	ticker := r.Clock.NewTicker(StatusUpdateTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Status update loop is stopping.")
			return
		case <-ticker.Chan():
			if err := r.updateStatus(ctx); err != nil {
				log.WithError(err).Warn("Failed to updates status.")
			}
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
	ctx, cancel := context.WithTimeout(ctx, StatusUpdateTimeout)
	defer cancel()

	systemStatus = &pb.SystemStatus{
		Status:    pb.SystemStatus_Unknown,
		Timestamp: pb.NewTimeToProto(r.Clock.Now()),
	}

	members, err := r.ClusterMembership.Members()
	if err != nil {
		log.WithError(err).Warn("Failed to query serf members.")
		return nil, trace.Wrap(err, "failed to query serf members")
	}

	log.Debugf("Started collecting statuses from members %v.", members)

	ctxNode, cancelNode := context.WithTimeout(ctx, nodeStatusTimeout)
	defer cancelNode()

	statusCh := make(chan *statusResponse, len(members))
	for _, member := range members {
		if r.Name == member.Name() {
			go r.getLocalStatus(ctxNode, statusCh)
		} else {
			go r.getStatusFrom(ctx, member, statusCh)
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
					status.member.Name(), status.member.Addr(), status.err)
				nodeStatus = unknownNodeStatus(status.member)
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
func (r *agent) collectLocalStatus(ctx context.Context) (status *pb.NodeStatus, err error) {
	local, err := r.ClusterMembership.FindMember(r.Name)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query local serf member")
	}

	status = r.runChecks(ctx)
	status.MemberStatus = statusFromMember(local)

	r.Lock()
	changes := history.DiffNode(r.Clock, r.localStatus, status)
	r.localStatus = status
	r.Unlock()

	/// TODO: handle recording of timeline outside of collection.
	if err := r.LocalTimeline.RecordEvents(ctx, changes); err != nil {
		return status, trace.Wrap(err, "failed to record local timeline events")
	}

	if err := r.notifyMasters(ctx); err != nil {
		return status, trace.Wrap(err, "failed to notify master nodes of local timeline events")
	}

	return status, nil
}

// getLocalStatus obtains local node status.
func (r *agent) getLocalStatus(ctx context.Context, respc chan<- *statusResponse) {
	// TODO: restructure code so that local member is not needed here.
	local, err := r.ClusterMembership.FindMember(r.Name)
	if err != nil {
		respc <- &statusResponse{err: err}
		return
	}

	status, err := r.collectLocalStatus(ctx)
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

// notifyMasters pushes new timeline events to all master nodes in the cluster.
func (r *agent) notifyMasters(ctx context.Context) error {
	members, err := r.ClusterMembership.Members()
	if err != nil {
		return trace.Wrap(err)
	}

	events, err := r.LocalTimeline.GetEvents(ctx, nil)
	if err != nil {
		return trace.Wrap(err)
	}

	// TODO: async
	for _, member := range members {
		if !hasRoleMaster(member.Tags()) {
			continue
		}
		if err := r.notifyMaster(ctx, member, events); err != nil {
			log.WithError(err).Warnf("Failed to notify %s of new timeline events.", member.Name())
		}
	}

	return nil
}

// notifyMaster push new timeline events to the specified member.
func (r *agent) notifyMaster(ctx context.Context, member membership.ClusterMember, events []*pb.TimelineEvent) error {
	client, err := member.Dial(ctx, r.CAFile, r.CertFile, r.KeyFile)
	if err != nil {
		return trace.Wrap(err)
	}
	defer client.Close()

	resp, err := client.LastSeen(ctx, &pb.LastSeenRequest{Name: r.Name})
	if err != nil {
		return trace.Wrap(err)
	}

	// Filter out previously recorded events.
	filtered := filterByTimestamp(events, resp.GetTimestamp().ToTime())

	for _, event := range filtered {
		if _, err := client.UpdateTimeline(ctx, &pb.UpdateRequest{Name: r.Name, Event: event}); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// getStatusFrom obtains node status from the node identified by member.
func (r *agent) getStatusFrom(ctx context.Context, member membership.ClusterMember, respc chan<- *statusResponse) {
	client, err := member.Dial(ctx, r.CAFile, r.CertFile, r.KeyFile)
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

// Status returns the last known cluster status.
func (r *agent) Status() (status *pb.SystemStatus, err error) {
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

// filterByTimestamp filters out events that occurred before the provided
// timestamp.
func filterByTimestamp(events []*pb.TimelineEvent, timestamp time.Time) (filtered []*pb.TimelineEvent) {
	for _, event := range events {
		if event.GetTimestamp().ToTime().Before(timestamp) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

// hasRoleMaster returns true if tags contains role 'master'.
func hasRoleMaster(tags map[string]string) bool {
	role, ok := tags["role"]
	return ok && Role(role) == RoleMaster
}

// statusResponse describes a status response from a background process that obtains
// health status on the specified serf node.
type statusResponse struct {
	*pb.NodeStatus
	member membership.ClusterMember
	err    error
}
