package sqlite

import "time"

const (
	// evictionFrequency is the time between eviction loops.
	evictionFrequency = time.Hour

	// evictionTimeout specifies the amount of time to given to evict events.
	evictionTimeout = 10 * time.Second
)

// These types are used to specify the type of an event when storing event
// into a database.
const (
	clusterRecoveredType = "ClusterRecovered"
	clusterDegradedType  = "ClusterDegraded"
	nodeAddedType        = "NodeAdded"
	nodeRemovedType      = "NodeRemoved"
	nodeRecoveredType    = "NodeRecovered"
	nodeDegradedType     = "NodeDegraded"
	probeSucceededType   = "ProbeSucceeded"
	probeFailedType      = "ProbeFailed"
	unknownType          = "Unknown"
)

// createTableEvents is sql statement to create an `events` table.
// Rows must be unique, excluding id.
// TODO: might not need oldState/newState.
const createTableEvents = `
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	type TEXT NOT NULL,
	node TEXT,
	probe TEXT,
	oldState TEXT,
	newState TEXT,
	UNIQUE(timestamp, type, node, probe, oldState, newState)
)
`

// TODO: index node/probe fields to improve filtering performance.

// insertIntoEvents is sql statement to insert entry into `events` table. Used for
// batch insert statement.
const insertIntoEvents = `
INSERT INTO events (
	timestamp,
	type,
	node,
	probe,
	oldState,
	newState
) VALUES (?,?,?,?,?,?)
`

// deleteOldFromEvents is sql statement to delete entries from `events` table.
const deleteOldFromEvents = `DELETE FROM events WHERE id IN (SELECT id FROM events WHERE timestamp < ?)`
