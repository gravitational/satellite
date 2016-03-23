package monitoring

import (
	"testing"

	. "gopkg.in/check.v1"
)

func TestCheckers(t *testing.T) { TestingT(t) }

type CheckersSuite struct{}

var _ = Suite(&CheckersSuite{})

func (_ *CheckersSuite) TestCreatesEtcdChecker(c *C) {
	config := &EtcdConfig{
		Endpoints: []string{"http://127.0.0.1:2379"},
	}
	checker, err := EtcdHealth(config)
	c.Assert(err, IsNil)
	c.Assert(checker, NotNil)
}

func (_ *CheckersSuite) TestCreatesEtcdCheckerWithTLS(c *C) {
	config := &EtcdConfig{
		Endpoints: []string{"https://127.0.0.1:2379"},
		TLSConfig: TLSConfig{
			CAFile:   "testdata/ca.pem",
			CertFile: "testdata/etcd.pem",
			KeyFile:  "testdata/etcd-key.pem",
		},
	}
	checker, err := EtcdHealth(config)
	c.Assert(err, IsNil)
	c.Assert(checker, NotNil)
}
