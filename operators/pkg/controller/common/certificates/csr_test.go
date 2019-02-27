// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestCertInitializerCSRClient_RetrieveCSR(t *testing.T) {
	expectedCSR := []byte("expected-csr")

	// test HTTP server
	fail := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != CertInitializerRoute || fail {
			w.WriteHeader(500)
			w.Write(nil)
			return
		}
		w.Write(expectedCSR)
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(tsURL.Port())
	require.NoError(t, err)

	// create a csr client
	csrClient := CertInitializerCSRClient{
		httpClient: http.Client{},
		route:      CertInitializerRoute,
		port:       port,
	}

	// should fail when pod does not have an IP
	_, err = csrClient.RetrieveCSR(corev1.Pod{})
	require.EqualError(t, err, "pod does not yet have an IP")

	// should succeed in the happy path
	csr, err := csrClient.RetrieveCSR(corev1.Pod{Status: corev1.PodStatus{PodIP: tsURL.Hostname()}})
	require.NoError(t, err)
	require.Equal(t, expectedCSR, csr)

	// should correctly fail if something is wrong with the server
	fail = true
	_, err = csrClient.RetrieveCSR(corev1.Pod{Status: corev1.PodStatus{PodIP: tsURL.Hostname()}})
	require.EqualError(t, err, "HTTP status code 500")

	// should correctly fail if requesting an IP with no server running
	_, err = csrClient.RetrieveCSR(corev1.Pod{Status: corev1.PodStatus{PodIP: "0.0.0.1"}})
	require.Error(t, err)
}
