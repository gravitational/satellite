package nethealth

import (
	"net"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jonboulle/clockwork"

	"github.com/stretchr/testify/assert"

	"github.com/sirupsen/logrus"

	"github.com/gravitational/trace"
)

func TestAck(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	buf := make([]byte, 1024)

	e := generateTestServerPair()
	go e.s1.loop()
	go e.s1.serve()
	e.s1.podList <- []string{"10.0.0.2"}

	//
	// have the server send a ping
	//
	e.clock1.Advance(1 * time.Second)
	n, _, err := e.conn.ReadFrom(buf)
	assert.NoError(t, err, "expect read from connection")
	hb, err := unmarshalHeartbeat(buf[:n])
	assert.NoError(t, err, "unmarshal error")
	assert.Equal(t, &agentpb.Heartbeat{
		Type:      agentpb.Heartbeat_REQUEST,
		Timestamp: e.clock1.Now().Format(time.RFC3339Nano),
		NodeName:  "node-1",
		RestartId: hb.RestartId,
		Count:     1,
	}, hb, "expected heartbeat")

	e.clock1.Advance(5 * time.Millisecond)
	// ack the heartbeat
	e.send(t, agentpb.Heartbeat{
		Type:      agentpb.Heartbeat_ACK,
		NodeName:  "node-2",
		Timestamp: hb.Timestamp,
	})

	e.sleep()
	log.Debug(spew.Sdump(promPacketRTT))
	metric, err := getFirstMetric(promPacketRTT.MetricVec)
	assert.NoError(t, err, "metric error")
	assert.Equal(t, uint64(1), *metric.Histogram.SampleCount, "latency samples")
	assert.Equal(t, "peer_name", *metric.Label[0].Name, "peer_name label name")
	assert.Equal(t, "node-2", *metric.Label[0].Value, "peer_name label value")

	// send a heartbeat and check the response
	req := agentpb.Heartbeat{
		Type:     agentpb.Heartbeat_REQUEST,
		NodeName: "node-2",
		// use a different clock time to ensure it's copied to the ack
		Timestamp: e.clock1.Now().Add(15 * time.Second).Format(time.RFC3339Nano),
	}
	e.send(t, req)

	n, _, err = e.conn.ReadFrom(buf)
	assert.NoError(t, err, "expect read from connection")
	hb, err = unmarshalHeartbeat(buf[:n])
	assert.NoError(t, err, "unmarshal error")
	assert.Equal(t, &agentpb.Heartbeat{
		Type:      agentpb.Heartbeat_ACK,
		Timestamp: req.Timestamp,
		NodeName:  "node-1",
	}, hb, "expected ack")
}

func TestPeerDown(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	buf := make([]byte, 1024)

	e := generateTestServerPair()
	go e.s1.loop()
	go e.s1.serve()
	e.s1.podList <- []string{"10.0.0.1", "10.0.0.2"}

	// ping the node, so it can learn the node name
	e.send(t, agentpb.Heartbeat{
		Type:      agentpb.Heartbeat_REQUEST,
		NodeName:  "node-2",
		Timestamp: e.clock1.Now().Format(time.RFC3339Nano),
	})

	// run 20 seconds of advancing the clock
	for i := 0; i < 20; i++ {
		e.advance(1 * time.Second)

		// read but discard the packet from the connection
		_, _, err := e.conn.ReadFrom(buf)
		assert.NoError(t, err, "expect read from connection")

	}

	// check that the peer has been marked as down
	metric, err := getFirstMetric(promPeerDown.MetricVec)
	assert.NoError(t, err, "metric error")

	assert.Equal(t, float64(1), *metric.Gauge.Value, "peer down")
	assert.Equal(t, "peer_name", *metric.Label[0].Name, "peer_name label name")
	assert.Equal(t, "node-2", *metric.Label[0].Value, "peer_name label value")

	// send a heartbeat so node-2 comes back up
	e.send(t, agentpb.Heartbeat{
		Type:      agentpb.Heartbeat_REQUEST,
		NodeName:  "node-2",
		Timestamp: e.clock1.Now().Format(time.RFC3339Nano),
		Count:     20,
	})

	e.advance(1 * time.Second)
	metric, err = getFirstMetric(promPeerDown.MetricVec)
	assert.NoError(t, err, "metric error")
	assert.Equal(t, float64(0), *metric.Gauge.Value, "peer up")
	assert.Equal(t, "peer_name", *metric.Label[0].Name, "peer_name label name")
	assert.Equal(t, "node-2", *metric.Label[0].Value, "peer_name label value")

	//
	// remove the pod from service discovery, ensure it gets marked as down
	//
	e.s1.podList <- []string{""}
	e.sleep()
	metric, err = getFirstMetric(promPeerDown.MetricVec)
	assert.NoError(t, err, "metric error")
	assert.Equal(t, float64(1), *metric.Gauge.Value, "peer down")
	assert.Equal(t, "peer_name", *metric.Label[0].Name, "peer_name label name")
	assert.Equal(t, "node-2", *metric.Label[0].Value, "peer_name label value")
}

func TestPacketLoss(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	buf := make([]byte, 1024)

	e := generateTestServerPair()
	go e.s1.loop()
	go e.s1.serve()
	e.s1.podList <- []string{"10.0.0.2"}

	for i := 0; i <= 20; i += 2 {
		e.send(t, agentpb.Heartbeat{
			Type:      agentpb.Heartbeat_REQUEST,
			NodeName:  "node-2",
			Timestamp: e.clock1.Now().Format(time.RFC3339Nano),
			Count:     int64(i),
			RestartId: "test",
		})

		// read but discard the packet from the connection
		_, _, err := e.conn.ReadFrom(buf)
		assert.NoError(t, err, "expect read from connection")
	}
	e.sleep()

	metric, err := getFirstMetric(promPacketLoss.MetricVec)
	assert.NoError(t, err, "metric error")
	assert.Equal(t, float64(10), *metric.Counter.Value, "packet loss")
	assert.Equal(t, "peer_name", *metric.Label[0].Name, "peer_name label name")
	assert.Equal(t, "node-2", *metric.Label[0].Value, "peer_name label value")
}

func TestFiltered(t *testing.T) {
	e := generateTestServerPair()
	go e.s1.serve()

	//
	// send a heartbeat from a disallowed range
	// and check that it's recorded in the metrics
	//
	e.conn.localAddr = &net.IPAddr{
		IP: net.ParseIP("1.1.1.1"),
	}
	e.send(t, agentpb.Heartbeat{})
	e.sleep()

	metric, err := getFirstMetric(promPacketFiltered.MetricVec)
	assert.NoError(t, err, "metric error")
	assert.Equal(t, float64(1), *metric.Counter.Value, "dropped packets")
	assert.Equal(t, "peer_addr", *metric.Label[0].Name, "peer_addr label name")
	assert.Equal(t, "1.1.1.1", *metric.Label[0].Value, "peer_addr label value")

}

func getFirstMetric(m *prometheus.MetricVec) (*dto.Metric, error) {
	metrics := make(chan prometheus.Metric, 10)
	metric := &dto.Metric{}
	m.Collect(metrics)
	err := (<-metrics).Write(metric)
	return metric, trace.Wrap(err)
}

type testEnvelope struct {
	clock1 clockwork.FakeClock
	s1     Server
	conn   MockPacketConn
}

func (e testEnvelope) send(t *testing.T, hb agentpb.Heartbeat) {
	resp, err := hb.Marshal()
	assert.NoError(t, err, "marshal error")
	_, err = e.conn.WriteTo(resp, nil)
	assert.NoError(t, err, "conn.WriteTo error")
}

func (e testEnvelope) advance(d time.Duration) {
	e.sleep()
	e.clock1.Advance(d)
	e.sleep()
}

// release the scheduler so other go-routines run
func (e testEnvelope) sleep() {
	// in local testing, I've been having issues with th scheduler when releasing for 0 milliseconds
	// so release for at least 1 millisecond to try and ensure other routines get scheduled
	time.Sleep(time.Millisecond)
}

func generateTestServerPair() testEnvelope {
	_, allowed, _ := net.ParseCIDR("10.0.0.0/8")

	conn1, conn2 := generatePacketConnPair()
	clock1 := clockwork.NewFakeClock()

	return testEnvelope{
		clock1: clock1,
		conn:   conn2,
		s1: Server{
			AllowedSubnets: []net.IPNet{*allowed},
			NodeName:       "node-1",
			PodIP:          "10.0.0.1",
			clock:          clock1,
			conn:           conn1,
			rxHeartbeat:    make(chan heartbeat, 10),
			podList:        make(chan []string),
			stopC:          make(chan bool),
		},
	}
}

func generatePacketConnPair() (MockPacketConn, MockPacketConn) {
	c1 := make(chan packet, 10)
	c2 := make(chan packet) // use synchronous channel from tester to avoid races

	a1 := &net.IPAddr{
		IP: net.ParseIP("10.0.0.1"),
	}
	a2 := &net.IPAddr{
		IP: net.ParseIP("10.0.0.2"),
	}

	return MockPacketConn{
			localAddr: a1,
			tx:        c1,
			rx:        c2,
		}, MockPacketConn{
			localAddr: a2,
			tx:        c2,
			rx:        c1,
		}
}

type MockPacketConn struct {
	localAddr net.Addr
	tx        chan packet
	rx        chan packet
}

type packet struct {
	dstAddr net.Addr
	srcAddr net.Addr
	buf     []byte
}

func (m MockPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	packet, more := <-m.rx
	if more {
		n = copy(p, packet.buf)
		addr = packet.srcAddr
		return
	}
	err = trace.ConnectionProblem(nil, "connection closed")
	return
}

func (m MockPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	m.tx <- packet{
		dstAddr: addr,
		srcAddr: m.localAddr,
		buf:     p,
	}
	return len(p), nil
}

func (m MockPacketConn) Close() error {
	close(m.tx)
	return nil
}

func (m MockPacketConn) LocalAddr() net.Addr {
	return m.localAddr
}

func (MockPacketConn) SetDeadline(t time.Time) error {
	return nil
}

func (MockPacketConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (MockPacketConn) SetWriteDeadline(t time.Time) error {
	return nil
}
