package agent

// import (
// 	"net"
// 	"net/http"

// 	"github.com/gravitational/satellite/utils"
// 	"github.com/gravitational/trace"
// 	"github.com/prometheus/client_golang/prometheus/promhttp"
// 	log "github.com/sirupsen/logrus"
// 	"golang.org/x/net/context"
// )

// type task func(context.Context) error

// // serveMetrics returns background task for serving prometheus metrics.
// // The metricss listener will be closed when the context is canceled.
// func (r *Agent) serveMetrics() func(context.Context) error {
// 	return func(ctx context.Context) (err error) {
// 		listener, err := net.Listen("tcp", r.MetricsAddr)
// 		if err != nil {
// 			return trace.Wrap(err, "failed to serve prometheus metrics")
// 		}
// 		defer listener.Close()

// 		// Register prometheus metrics handler.
// 		http.Handle("/metrics", promhttp.Handler())

// 		errCh := make(chan error, 0)
// 		go func() {
// 			errCh <- http.Serve(listener, nil)
// 		}()

// 		// Wait for context to be done or Serve to return.
// 		select {
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		case errServe := <-errCh:
// 			if errServe != nil && errServe != http.ErrServerClosed {
// 				return trace.Wrap(err)
// 			}
// 		}

// 		log.Debug("Metrics listener has been shutdown/closed.")
// 		return nil
// 	}
// }

// // recycleLoop periodically recycles the cache.
// func (r *Agent) recycleLoop(ctx context.Context) (err error) {
// 	ticker := r.Clock.NewTicker(recycleInterval)
// 	defer ticker.Stop()
// 	for range ticker.Chan() {
// 		if utils.IsContextDone(ctx) {
// 			log.Info("Recycle loop is stopping.")
// 			return
// 		}
// 		if err := r.Cache.Recycle(); err != nil {
// 			log.WithError(err).Warnf("Error recycling status.")
// 			continue
// 		}
// 	}
// 	return
// }

// // statusUpdateLoop is a long running background process that periodically
// // updates the health status of the cluster by querying status of other active
// // cluster members.
// func (r *Agent) statusUpdateLoop(ctx context.Context) error {
// 	ticker := r.Clock.NewTicker(statusUpdateInterval)
// 	defer ticker.Stop()
// 	for range ticker.Chan() {
// 		if utils.IsContextDone(ctx) {
// 			log.Info("Status update loop is stopping.")
// 			return nil
// 		}
// 		if err := r.updateStatus(ctx); err != nil {
// 			log.WithError(err).Warnf("Failed to updates status.")
// 			continue
// 		}
// 	}
// 	return nil
// }

// // subscriberLoop periodically updates the set of subscribers to this agent's
// // event queue.
// func (r *Agent) subscriberLoop(ctx context.Context) error {
// 	ticker := r.Clock.NewTicker(subscriberInterval)
// 	defer ticker.Stop()
// 	for range ticker.Chan() {
// 		if utils.IsContextDone(ctx) {
// 			log.Info("Subscriber loop is stopping.")
// 			return nil
// 		}
// 		if err := r.subscriberUpdate(); err != nil {
// 			log.WithError(err).Warnf("Failed to update subscribers.")
// 			continue
// 		}
// 	}
// 	return nil
// }

// // subscriberUpdate updates the current set of subscribers. If any new master
// // nodes have joined the cluster, they will be subscribed to this agent's event
// // queue.
// func (r *Agent) subscriberUpdate() error {
// 	members, err := r.SerfClient.Members()
// 	if err != nil {
// 		return trace.Wrap(err)
// 	}
// 	for _, member := range members {
// 		// Only subscribe master nodes.
// 		if role, ok := member.Tags["role"]; !ok || Role(role) != RoleMaster {
// 			continue
// 		}

// 		// Member is already subscribed.
// 		if r.Queue.IsSubscribed(member.Name) {
// 			continue
// 		}

// 		if err := r.subscribeMember(member); err != nil {
// 			return trace.Wrap(err)
// 		}
// 	}
// 	return nil
// }
