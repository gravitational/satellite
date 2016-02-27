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
package sqlite

import (
	"database/sql"
	"time"

	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
)

type backend struct {
	*sql.DB
	clock clockwork.Clock
	done  chan struct{}
}

const schema = `
PRAGMA foreign_keys = TRUE;

CREATE TABLE IF NOT EXISTS node (
  id INTEGER PRIMARY KEY NOT NULL,
  name TEXT UNIQUE,
  member_addr TEXT
);

-- key/value store
CREATE TABLE IF NOT EXISTS tag (
  id INTEGER PRIMARY KEY NOT NULL,
  key TEXT,
  value TEXT,
  UNIQUE (key, value) ON CONFLICT IGNORE
);

CREATE TABLE IF NOT EXISTS node_tag (
  node_id INTEGER,
  tag_id INTEGER,
  FOREIGN KEY(node_id) REFERENCES node(id),
  FOREIGN KEY(tag_id) REFERENCES tag(id),
  UNIQUE (node_id, tag_id) ON CONFLICT IGNORE
);

CREATE VIEW IF NOT EXISTS node_tags AS
SELECT n.name AS node_name, t.key, t.value
FROM node_tag nt
  INNER JOIN node n ON nt.node_id = n.id
  INNER JOIN tag t ON nt.tag_id = t.id;

CREATE TRIGGER IF NOT EXISTS insert_tags
INSTEAD OF INSERT ON node_tags
WHEN EXISTS (SELECT 1 FROM node WHERE node.name = new.node_name)
BEGIN
  INSERT OR IGNORE INTO tag(key, value) VALUES(new.key, new.value);

  INSERT OR IGNORE INTO node_tag(node_id, tag_id)
  SELECT n.id, t.id FROM node n JOIN tag t
  WHERE n.name = new.node_name AND t.key = new.key AND t.value = new.value;
END;

-- history of system status changes
CREATE TABLE IF NOT EXISTS system_snapshot (
  id INTEGER PRIMARY KEY NOT NULL,
  -- system status: running/degraded
  status CHAR(1) CHECK(status IN ('H', 'F')) NOT NULL DEFAULT 'H',
  summary TEXT,
  captured_at TIMESTAMP NOT NULL UNIQUE
);

-- history of node status changes
CREATE TABLE IF NOT EXISTS node_snapshot (
  node_id INTEGER,
  snapshot_id INTEGER,
  -- node status: running/degraded
  status CHAR(1) CHECK(status IN ('H', 'F')) NOT NULL DEFAULT 'H',
  -- serf member status: alive/leaving/left/failed
  member_status CHAR(1)	CHECK(member_status IN ('A', 'G', 'L', 'F')) NOT NULL DEFAULT 'A',
  FOREIGN KEY(node_id) REFERENCES node(id) ON DELETE CASCADE,
  FOREIGN KEY(snapshot_id) REFERENCES system_snapshot(id) ON DELETE CASCADE,
  UNIQUE (node_id, snapshot_id) ON CONFLICT IGNORE
);

-- history of monitoring test probes
CREATE TABLE IF NOT EXISTS probe (
  node_id INTEGER NOT NULL,
  snapshot_id INTEGER,
  checker TEXT NOT NULL,
  detail TEXT,
  -- running/failed/terminated
  status CHAR(1) CHECK(status IN ('H', 'F', 'T')) NOT NULL DEFAULT 'F',
  error	TEXT NOT NULL,
  FOREIGN KEY(node_id) REFERENCES node(id) ON DELETE CASCADE,
  FOREIGN KEY(snapshot_id) REFERENCES system_snapshot(id) ON DELETE CASCADE
);

CREATE VIEW IF NOT EXISTS system_status AS
SELECT s.id, s.captured_at, s.status AS cluster_status, s.summary, ns.status AS node_status, ns.member_status,
	n.name, n.member_addr, p.checker, p.detail, p.status, p.error
FROM system_snapshot s
  INNER JOIN node_snapshot ns ON ns.snapshot_id = s.id AND ns.node_id = n.id
  INNER JOIN node n ON ns.node_id = n.id
  LEFT OUTER JOIN probe p ON p.node_id = n.id AND p.snapshot_id = s.id;

CREATE TRIGGER IF NOT EXISTS insert_system_status
INSTEAD OF INSERT ON system_status
BEGIN
  INSERT OR IGNORE INTO system_snapshot(status, summary, captured_at)
  VALUES(new.cluster_status, new.summary, new.captured_at);

  INSERT OR IGNORE INTO node(name, member_addr) VALUES(new.name, new.member_addr);
  UPDATE node SET member_addr = new.member_addr WHERE name = new.name AND new.member_addr IS NOT NULL;
  
  INSERT OR IGNORE INTO node_snapshot(node_id, snapshot_id, status, member_status)
  SELECT n.id as node_id, s.id as snapshot_id, new.node_status, new.member_status
  FROM node n JOIN system_snapshot s WHERE n.name = new.name AND s.captured_at = new.captured_at;
  
  INSERT INTO probe(node_id, snapshot_id, checker, detail, status, error)
  SELECT n.id, s.id, new.checker, new.detail, new.status, new.error
  FROM node n JOIN system_snapshot s
  WHERE n.name = new.name AND s.captured_at = new.captured_at AND new.checker IS NOT NULL;
END;
`

// New creates a new sqlite backend using the specified file.
func New(path string) (*backend, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	clock := clockwork.NewRealClock()
	return newBackend(db, clock)
}

// Update creates a new snapshot of the system state specified with status.
func (r *backend) UpdateStatus(status *pb.SystemStatus) (err error) {
	if err = inTx(r.DB, func(tx *sql.Tx) error {
		const insertStatus = `
			INSERT INTO system_status(captured_at, cluster_status, summary, node_status, member_status,
						  name, member_addr, checker, detail, status, error)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		const insertStatusNoProbe = `
			INSERT INTO system_status(captured_at, cluster_status, summary, node_status, member_status,
						  name, member_addr)
			VALUES(?, ?, ?, ?, ?, ?, ?)
		`
		const insertTags = `INSERT INTO node_tags(node_name, key, value) VALUES(?, ?, ?)`

		summary := nullString(status.Summary)
		for _, node := range status.Nodes {
			memberAddr := nullString(node.MemberStatus.Addr)
			for _, probe := range node.Probes {
				if _, err = tx.Exec(insertStatus,
					(*timestamp)(status.Timestamp),
					protoToSystemStatus(status.Status),
					summary,
					protoToNodeStatus(node.Status),
					protoToMemberStatus(node.MemberStatus.Status),
					node.Name, memberAddr,
					probe.Checker, probe.Detail, protoToProbe(probe.Status),
					probe.Error); err != nil {
					return trace.Wrap(err)
				}
			}
			// Persist node status even for nodes w/o probes
			if len(node.Probes) == 0 {
				if _, err = tx.Exec(insertStatusNoProbe,
					(*timestamp)(status.Timestamp),
					protoToSystemStatus(status.Status),
					status.Summary,
					protoToNodeStatus(node.Status),
					protoToMemberStatus(node.MemberStatus.Status),
					node.Name, node.MemberStatus.Addr); err != nil {
					return trace.Wrap(err)
				}
			}
			for key, value := range node.MemberStatus.Tags {
				if _, err := tx.Exec(insertTags, node.Name, key, value); err != nil {
					return trace.Wrap(err)
				}
			}
		}

		return nil
	}); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// RecentStatus obtains the last known cluster status.
func (r *backend) RecentStatus() (*pb.SystemStatus, error) {
	const selectEverything = `
	SELECT s.cluster_status, s.summary, s.captured_at,
	  s.name as node_name, s.node_status, s.member_status, s.member_addr,
	  s.checker, s.detail, s.status, s.error
	FROM system_status s WHERE s.id in (SELECT max(id) FROM system_snapshot)
	ORDER BY node_name
	`
	const selectTags = `
	SELECT nt.node_name, nt.key, nt.value
	FROM node_tags nt
	  INNER JOIN node n ON nt.node_name = n.name
	  INNER JOIN node_snapshot ns ON ns.node_id = n.id
	  INNER JOIN system_snapshot s ON ns.snapshot_id = s.id
	WHERE s.captured_at = ?
	`

	var status *pb.SystemStatus
	if err := r.selector(selectEverything, statusSelector(&status)); err != nil {
		return nil, trace.Wrap(err)
	}
	if status != nil {
		ts := (*timestamp)(status.Timestamp)
		if err := r.selector(selectTags, tagSelector(status), ts); err != nil {
			return nil, trace.Wrap(err)
		}
	} else {
		status = &pb.SystemStatus{Status: pb.SystemStatus_Unknown}
	}
	return status, nil
}

// Close closes the database and releases resources.
func (r *backend) Close() error {
	close(r.done)
	return r.DB.Close()
}

// selector queries the backend using the specified query and accumulates results
// using the provided accumulator function.
func (r *backend) selector(selectStmt string, f accumulator, args ...interface{}) error {
	rows, err := r.Query(selectStmt, args...)
	if err != nil {
		return trace.Wrap(err)
	}
	defer rows.Close()

	if err = f(rows); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// accumulator defines an interface to obtain results from the specified rows.
type accumulator func(rows *sql.Rows) error

// statusSelector returns an accumulator that interprets a system status query.
func statusSelector(status **pb.SystemStatus) accumulator {
	return func(rows *sql.Rows) error {
		var node *pb.NodeStatus
		for rows.Next() {
			var (
				ts           timestamp
				summary      sql.NullString
				systemStatus systemStatusType
				nodeName     string
				nodeStatus   nodeStatusType
				memberStatus memberStatusType
				memberAddr   sql.NullString
				checker      sql.NullString
				detail       sql.NullString
				probeStatus  probeType
				probeMessage sql.NullString
			)
			if err := rows.Scan(&systemStatus, &summary, &ts,
				&nodeName, &nodeStatus, &memberStatus, &memberAddr,
				&checker, &detail, &probeStatus, &probeMessage); err != nil {
				return trace.Wrap(err)
			}
			if *status == nil {
				*status = &pb.SystemStatus{
					Status:    systemStatus.toProto(),
					Timestamp: (*pb.Timestamp)(&ts),
					Summary:   summary.String,
				}
			}
			if node != nil && node.Name != nodeName {
				(*status).Nodes = append((*status).Nodes, node)
				node = nil
			}
			if node == nil {
				node = &pb.NodeStatus{
					Name:   nodeName,
					Status: nodeStatus.toProto(),
					MemberStatus: &pb.MemberStatus{
						Name:   nodeName,
						Addr:   memberAddr.String,
						Status: memberStatus.toProto(),
					},
				}
			}
			if checker.Valid {
				node.Probes = append(node.Probes, &pb.Probe{
					Checker: checker.String,
					Detail:  detail.String,
					Status:  probeStatus.toProto(),
					Error:   probeMessage.String,
				})
			}
		}
		if node != nil {
			(*status).Nodes = append((*status).Nodes, node)
		}
		if rows.Err() != nil {
			return trace.Wrap(rows.Err())
		}
		return nil
	}
}

// tagSelector returns an accumulator that interprets a query for node tags.
func tagSelector(status *pb.SystemStatus) accumulator {
	nodeNameToIndex := func(name string) int {
		for i, node := range status.Nodes {
			if node.Name == name {
				return i
			}
		}
		return -1
	}
	return func(rows *sql.Rows) error {
		for rows.Next() {
			var nodeName, key, value string
			err := rows.Scan(&nodeName, &key, &value)
			if err != nil {
				return trace.Wrap(err)
			}
			index := nodeNameToIndex(nodeName)
			if index >= 0 {
				memberStatus := status.Nodes[index].MemberStatus
				if memberStatus.Tags == nil {
					memberStatus.Tags = make(map[string]string)
				}
				memberStatus.Tags[key] = value
			}
		}
		return rows.Err()
	}
}

// scavengeTimeout is the amount of time between cleanup iterations.
const scavengeTimeout = 24 * time.Hour

// scavengeLoop is the background process to cleanup time series values older than
// the specified threshold.
func (r *backend) scavengeLoop() {
	for {
		select {
		case <-r.clock.After(scavengeTimeout):
			if err := r.deleteOlderThan(r.clock.Now().Add(-scavengeTimeout)); err != nil {
				log.Errorf("failed to scavenge stats: %v", err)
			}
		case <-r.done:
			return
		}
	}
}

// deleteOlderThan removes items older than the specified time limit.
func (r *backend) deleteOlderThan(limit time.Time) error {
	const deleteStmt = `DELETE FROM system_snapshot WHERE captured_at < ?`

	if err := inTx(r.DB, func(tx *sql.Tx) error {
		_, err := tx.Exec(deleteStmt, timestamp(pb.TimeToProto(limit)))
		if err != nil {
			return trace.Wrap(err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func inTx(db *sql.DB, f func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		tx.Commit()
	}()
	err = f(tx)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func newInMemory(clock clockwork.Clock) (*backend, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return newBackend(db, clock)
}

func newBackend(db *sql.DB, clock clockwork.Clock) (*backend, error) {
	_, err := db.Exec(schema)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create schema")
	}

	backend := &backend{
		DB:    db,
		clock: clock,
		done:  make(chan struct{}),
	}
	go backend.scavengeLoop()
	return backend, nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{
		String: value,
		Valid:  value != "",
	}
}
