/*
Copyright 2017 Gravitational, Inc.

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

package utils

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"

	"github.com/gravitational/trace"
)

// CertPoolFromFile returns an x509.CertPool containing the certificates
// in the given PEM-encoded file.
// Returns an error if the file could not be read, a certificate could not
// be parsed, or if the file does not contain any certificates
func CertPoolFromFile(filename string) (*x509.CertPool, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(b) {
		return nil, trace.Wrap(err)
	}
	return cp, nil
}

// CertFromFilePair returns an tls.Certificate containing the
// certificates public/private key pair from a pair of given PEM-encoded files.
// Returns an error if the file could not be read, a certificate could not
// be parsed, or if the file does not contain any certificates
func CertFromFilePair(certFile, keyFile string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &cert, err
}

// AddCertToDefaultPool adds certificate from file to default pool
func AddCertToDefaultPool(fpath string) error {
	certBytes, err := ioutil.ReadFile(fpath)
	if err != nil {
		return trace.Wrap(err)
	}
	tr := http.DefaultTransport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{
		ClientCAs: x509.NewCertPool(),
	}
	if ok := tr.TLSClientConfig.ClientCAs.AppendCertsFromPEM(certBytes); !ok {
		return trace.BadParameter("failed to load PEM %s", fpath)
	}
	return nil
}

// NewServerTLS constructs a TLS from the input certificate file, key
// file for server and certificate file for client verification.
func NewServerTLS(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := CertFromFilePair(certFile, keyFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	cp, err := CertPoolFromFile(caFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
		ClientCAs:    cp,
	}, nil
}
