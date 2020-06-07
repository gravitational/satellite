module github.com/gravitational/satellite

go 1.13

require (
	github.com/armon/go-metrics v0.0.0-20190430140413-ec5e00d3c878 // indirect
	github.com/aws/aws-sdk-go v1.25.41
	github.com/blang/semver v3.5.1+incompatible
	github.com/cloudfoundry/gosigar v1.1.1-0.20180406153506-1375283248c3
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/coreos/go-systemd/v22 v22.1.0
	github.com/davecgh/go-spew v1.1.1
	github.com/dustin/go-humanize v0.0.0-20171111073723-bb3d318650d4
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d
	github.com/golang/protobuf v1.3.2
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gravitational/configure v0.0.0-20161002181724-4e0f2df8846e
	github.com/gravitational/log v0.0.0-20200127200505-fdffa14162b0 // indirect
	github.com/gravitational/roundtrip v1.0.0
	github.com/gravitational/trace v1.1.10-0.20200129130229-dd5b2e8eae86
	github.com/gravitational/ttlmap v0.0.0-20171116003245-91fd36b9004c
	github.com/gravitational/version v0.0.2-0.20170324200323-95d33ece5ce1
	github.com/hashicorp/go-immutable-radix v1.1.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/serf v0.9.2
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/influxdata/influxdb v1.5.1
	github.com/jmoiron/sqlx v1.2.0
	github.com/jonboulle/clockwork v0.1.1-0.20190114141812-62fb9bc030d1
	github.com/kr/pretty v0.2.0 // indirect
	github.com/kylelemons/godebug v0.0.0-20170820004349-d65d576e9348
	github.com/magefile/mage v1.8.0
	github.com/mattn/go-sqlite3 v1.13.0
	github.com/miekg/dns v1.1.26
	github.com/mitchellh/go-ps v1.0.0
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.6.0
	github.com/prometheus/procfs v0.0.5
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20200602225109-6fdc65e7d980
	google.golang.org/grpc v1.23.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	k8s.io/api v0.17.6
	k8s.io/apimachinery v0.17.6
	k8s.io/client-go v0.17.6
)

replace github.com/gravitational/ttlmap => github.com/a-palchikov/ttlmap v0.0.0-20200607134314-5bbd378b94a2
