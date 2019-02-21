// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

const (
	// CertInitializerRoute is the HTTP route to request on the cert-initializer container.
	CertInitializerRoute = "/csr"
	// CSRRequestTimeout specifies how long we should wait for the CSR request.
	CSRRequestTimeout = 10 * time.Second
)

// CSRClient allows retrieving a CSR for a pod.
type CSRClient interface {
	RetrieveCSR(pod corev1.Pod) ([]byte, error)
}

// CertInitializerCSRClient requests a cert-initializer init container
// HTTP endpoint to retrieve the CSR.
type CertInitializerCSRClient struct {
	httpClient http.Client
	route      string
	port       int
}

// NewCertInitializerCSRClient creates a CertInitializerCSRClient.
func NewCertInitializerCSRClient(dialer net.Dialer, timeout time.Duration) CSRClient {
	transport := &http.Transport{}
	if dialer != nil {
		transport.DialContext = dialer.DialContext
	}
	return CertInitializerCSRClient{
		httpClient: http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		route: CertInitializerRoute,
		port:  initcontainer.CertInitializerPort,
	}
}

// RetrieveCSR retrieves the CSR by requesting the given pod's
// cert-initializer init container HTTP endpoint.
func (c CertInitializerCSRClient) RetrieveCSR(pod corev1.Pod) ([]byte, error) {
	if pod.Status.PodIP == "" {
		return nil, errors.New("pod does not yet have an IP")
	}
	url := fmt.Sprintf("http://%s:%d%s", pod.Status.PodIP, c.port, c.route)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP status code %d", resp.StatusCode)
	}

	return ioutil.ReadAll(resp.Body)
}
