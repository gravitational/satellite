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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/gravitational/trace"
)

// EtcdConfig defines a set of configuration parameters for accessing
// etcd endpoints
type EtcdConfig struct {
	// Endpoints lists etcd server endpoints
	Endpoints []string
	// CAFile is an SSL Certificate Authority file used to secure
	// communication with etcd
	CAFile string
	// CertFile is an SSL certificate file used to secure
	// communication with etcd
	CertFile string
	// KeyFile is an SSL key file used to secure communication with etcd
	KeyFile string
}

// etcdChecker is an HttpResponseChecker that interprets results from
// an etcd HTTP-based healthz end-point.
func etcdChecker(response io.Reader) error {
	payload, err := ioutil.ReadAll(response)
	if err != nil {
		return trace.Wrap(err)
	}

	healthy, err := etcdStatus(payload)
	if err != nil {
		return trace.Wrap(err)
	}

	if !healthy {
		return trace.Errorf("unexpected etcd status: %s", payload)
	}
	return nil
}

// etcdStatus determines if the specified etcd status value
// indicates a healthy service
func etcdStatus(payload []byte) (healthy bool, err error) {
	result := struct{ Health string }{}
	nresult := struct{ Health bool }{}
	err = json.Unmarshal(payload, &result)
	if err != nil {
		err = json.Unmarshal(payload, &nresult)
	}
	if err != nil {
		return false, trace.Wrap(err)
	}

	return (result.Health == "true" || nresult.Health == true), nil
}

// newHttpTransport creates a new http.Transport from the specified
// set of attributes.
// The resulting transport can be used to create an http.Client
func (r *EtcdConfig) newHttpTransport() (*http.Transport, error) {
	tlsConfig, err := r.clientConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 500,
		TLSClientConfig:     tlsConfig,
	}

	return transport, nil
}

// clientConfig generates a tls.Config object for use by an HTTP client.
func (r *EtcdConfig) clientConfig() (*tls.Config, error) {
	if r.empty() {
		return nil, nil
	}
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

// Empty determines if the configuration does not reference any certificate/key
// files
func (r *EtcdConfig) empty() bool {
	return r.CAFile == "" && r.CertFile == "" && r.KeyFile == ""
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
