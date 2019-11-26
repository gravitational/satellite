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

package history

import (
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func TestHistory(t *testing.T) { TestingT(t) }

type HistorySuite struct{}

var _ = Suite(&HistorySuite{})

// removeTimestamp removes timestamp from events. Events save timestamp on
// initialization. Easier to test by just removing timestamps.
func removeTimestamps(events []*Event) {
	for _, event := range events {
		event.timestamp = time.Time{}
	}
}
