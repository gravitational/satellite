package monitoring

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"github.com/gravitational/trace"
)

// This file implements client TLS configuration for HTTP.
// It is based on github.com/coreos/etcd/pkg/transport/listener.go

type TLSConfig struct {
	CAFile   string
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
	config.RootCAs, err = newCertPool([]string{r.CAFile})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return config, nil
}

// newCertPool creates x509 certPool with provided CA files.
func newCertPool(CAFiles []string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()

	for _, CAFile := range CAFiles {
		pemByte, err := ioutil.ReadFile(CAFile)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		for {
			var block *pem.Block
			block, pemByte = pem.Decode(pemByte)
			if block == nil {
				break
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			certPool.AddCert(cert)
		}
	}

	return certPool, nil
}
