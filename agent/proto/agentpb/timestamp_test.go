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

package agentpb

import (
	"time"

	. "gopkg.in/check.v1"
)

func (_ *ProtoSuite) TestWrapsTime(c *C) {
	expected := time.Now().UTC()
	ts := TimeToProto(expected)
	actual := ts.ToTime()
	c.Assert(expected, DeepEquals, actual)
}

func (_ *ProtoSuite) TestMarshalsTime(c *C) {
	expected := time.Now().UTC()
	ts := TimeToProto(expected)
	text, err := ts.MarshalText()
	c.Assert(err, IsNil)

	ts2 := Timestamp{}
	c.Assert(ts2.UnmarshalText(text), IsNil)

	c.Assert(ts, DeepEquals, ts2)
}
