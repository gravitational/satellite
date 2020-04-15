/*
Copyright 2019 Gravitational, Inc.

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

import "time"

const (
	// defaultTimelineRetention defines the default duration to store timeline events.
	defaultTimelineRentention = time.Hour * 24 * 7

	// evictionFrequency is the time between eviction loops.
	evictionFrequency = time.Hour

	// evictionTimeout specifies the amount of time given to evict events.
	evictionTimeout = 10 * time.Second
)

// createTableEvents is sql statement to create an `events` table.
// Rows must be unique, excluding id.
const createTableEvents = `
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
	type TEXT NOT NULL,
	node TEXT DEFAULT '',
	probe TEXT DEFAULT '',
	oldState TEXT DEFAULT '',
	newState TEXT DEFAULT '',
	UNIQUE(timestamp, type, node, probe, oldState, newState)
)
`

// deleteOldFromEvents is sql statement to delete entries from `events` table.
const deleteOldFromEvents = `DELETE FROM events WHERE timestamp < ?`
