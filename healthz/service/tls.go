package service

import (
	"io/ioutil"
	"fmt"
	"crypto/tls"
	"crypto/x509"
)

// CertPoolFromFile returns an x509.CertPool containing the certificates
// in the given PEM-encoded file.
// Returns an error if the file could not be read, a certificate could not
// be parsed, or if the file does not contain any certificates
func CertPoolFromFile(filename string) (*x509.CertPool, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("can't read CA file: %v", filename)
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(b) {
		return nil, fmt.Errorf("failed to append certificates from file: %s", filename)
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
		return nil, fmt.Errorf("can't load key pair from cert %s and key %s", certFile, keyFile)
	}
	return &cert, err
}

// NewServerTLS constructs a TLS from the input certificate file, key
// file for server and certificate file for client verification.
func NewServerTLS(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := CertFromFilePair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cp, err := CertPoolFromFile(caFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
		ClientCAs:    cp,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}