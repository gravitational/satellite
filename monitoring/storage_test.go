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

package monitoring

import (
	"context"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/gravitational/satellite/agent/health"

	"github.com/stretchr/testify/require"
)

const (
	shallSucceed = true
	shallFail    = false
)

func TestStorage(t *testing.T) {
	tmp, err := fsFromPath("/tmp")
	require.NoError(t, err)

	t.Logf("%+v", tmp)

	StorageChecker{
		Path: "/tmp",
	}.probe(t, "no conditions", shallSucceed)

	StorageChecker{
		Path:              path.Join("/tmp", "do-not-exist"),
		WillBeCreated:     true,
		Filesystems:       []string{tmp.SysTypeName},
		MinFreeBytes:      uint64(1e16),
		MinBytesPerSecond: uint64(1e16),
	}.probe(t, "unreasonable requirements", shallFail)

	StorageChecker{
		Path:              path.Join("/tmp", "do-not-exist"),
		WillBeCreated:     true,
		Filesystems:       []string{tmp.SysTypeName},
		MinFreeBytes:      uint64(1e5),
		MinBytesPerSecond: uint64(1e5),
	}.probe(t, "reasonable requirements", shallSucceed)

	StorageChecker{
		Path:          path.Join("/tmp", fmt.Sprintf("%d", time.Now().Unix())),
		WillBeCreated: true,
		Filesystems:   []string{"no-such-fs"},
	}.probe(t, "fs type mismatch", shallFail)

	StorageChecker{
		Path:          path.Join("/tmp", fmt.Sprintf("%d", time.Now().Unix())),
		WillBeCreated: false,
	}.probe(t, "missing folder", shallFail)

	StorageChecker{
		Path:          path.Join("/tmp", fmt.Sprintf("%d", time.Now().Unix())),
		WillBeCreated: true,
	}.probe(t, "create if missing", shallSucceed)
}

func (ch StorageChecker) probe(t *testing.T, msg string, success bool) {
	var probes health.Probes

	ch.Check(context.TODO(), &probes)

	failed := probes.GetFailed()
	if success && len(failed) == 0 {
		return
	}

	if success != (len(failed) == 0) {
		t.Fail()
	}

	t.Logf("%q failed probes:\n", msg)
	for i, probe := range failed {
		t.Logf("[%d] %s: %s %s\n",
			i+1, probe.Checker, probe.Detail, probe.Error)
	}
}
