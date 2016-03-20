package monitoring

import (
	"crypto/tls"
	"io/ioutil"

	"github.com/gravitational/trace"
)

// This file implements client TLS configuration for HTTP.
// It is based on github.com/coreos/etcd/pkg/transport/listener.go

type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// ClientConfig generates a tls.Config object for use by an HTTP client.
func (r *TLSConfig) ClientConfig() (*tls.Config, error) {
	cert, err := ioutil.ReadFile(r.CertFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	key, err := ioutil.ReadFile(r.KeyFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS10,
	}
	return config, nil
}
