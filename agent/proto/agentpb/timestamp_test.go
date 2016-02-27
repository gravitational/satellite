package agentpb

import (
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

func TestTimestamp(t *testing.T) { TestingT(t) }

type TimestampSuite struct{}

var _ = Suite(&TimestampSuite{})

func (_ *TimestampSuite) TestWrapsTime(c *C) {
	expected := time.Now().UTC()
	ts := TimeToProto(expected)
	actual := ts.ToTime()
	c.Assert(expected, DeepEquals, actual)
}

func (_ *TimestampSuite) TestMarshalsTime(c *C) {
	expected := time.Now().UTC()
	ts := TimeToProto(expected)
	text, err := ts.MarshalText()
	c.Assert(err, IsNil)

	ts2 := Timestamp{}
	c.Assert(ts2.UnmarshalText(text), IsNil)

	c.Assert(ts, DeepEquals, ts2)
}
