// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package http

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

// Client returns an http.Client configured for targeting a service managed by ECK.
// Features:
// - use the custom dialer if provided (can be nil) for eg. custom port-forwarding
// - use the provided ca certs for TLS verification (can be nil)
// - verify TLS certs, but ignore the server name: users may provide their own TLS certificate that may not
// match Kubernetes internal service name, but only the user-facing public endpoint
// - set APM spans with each request
func Client(dialer net.Dialer, caCerts []*x509.Certificate, timeout time.Duration) *http.Client {
	transportConfig := http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, // this is the default as of Go 1.18 we are just restating this here for clarity.

			// We use our own certificate verification because we permit users to provide their own certificates, which may not
			// be valid for the k8s service URL (though our self-signed certificates are). For instance, users may use a certificate
			// issued by a public CA. We opt to skip verifying here since we're not validating based on DNS names
			// or IP addresses, which means we have to do our own verification in VerifyPeerCertificate instead.

			// go requires either ServerName or InsecureSkipVerify (or both) when handshaking as a client since 1.3:
			// https://github.com/golang/go/commit/fca335e91a915b6aae536936a7694c4a2a007a60
			InsecureSkipVerify: true, //nolint:gosec
			VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
				return errors.New("tls: verify peer certificate not setup")
			},
		},
	}

	// only replace default cert pool if we have certificates to trust
	if caCerts != nil {
		certPool := x509.NewCertPool()
		for _, c := range caCerts {
			certPool.AddCert(c)
		}
		transportConfig.TLSClientConfig.RootCAs = certPool
	}

	transportConfig.TLSClientConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if verifiedChains != nil {
			return errors.New("tls: non-nil verifiedChains argument breaks crypto/tls.Config.VerifyPeerCertificate contract")
		}
		_, _, err := cryptutil.VerifyCertificateExceptServerName(rawCerts, transportConfig.TLSClientConfig)
		return err
	}

	// use the custom dialer if provided
	if dialer != nil {
		transportConfig.DialContext = dialer.DialContext
	}

	return &http.Client{
		Transport: &transportConfig,
		Timeout:   timeout,
	}
}

// APIError to represent non-200 HTTP responses as Go errors.
type APIError struct {
	StatusCode int
	msg        string
}

// MaybeAPIError creates an APIError from a http.Response if the status code is not 2xx.
func MaybeAPIError(resp *http.Response) *APIError {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		url := "unknown url"
		if resp.Request != nil {
			url = resp.Request.URL.Redacted()
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			msg:        fmt.Sprintf("failed to request %s, status is %d)", url, resp.StatusCode),
		}
	}
	return nil
}

func (e *APIError) Error() string {
	return e.msg
}

// IsNotFound checks whether the error was an HTTP 404 error.
func IsNotFound(err error) bool {
	return isHTTPError(err, http.StatusNotFound)
}

// IsUnauthorized checks whether the error was an HTTP 401 error.
func IsUnauthorized(err error) bool {
	return isHTTPError(err, http.StatusUnauthorized)
}

// IsForbidden checks whether the error was an HTTP 403 error.
func IsForbidden(err error) bool {
	return isHTTPError(err, http.StatusForbidden)
}

func isHTTPError(err error, statusCode int) bool {
	apiErr := new(APIError)
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == statusCode
	}
	return false
}
