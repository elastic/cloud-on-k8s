// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package epr

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

type APIError struct {
	StatusCode int
	msg        string
}

func (e *APIError) Error() string {
	return e.msg
}

func NewEPRClient(epr v1alpha1.PackageRegistry, k *test.K8sClient) (*http.Client, error) {
	var caCerts []*x509.Certificate
	if epr.Spec.HTTP.TLS.Enabled() {
		crts, err := k.GetHTTPCerts(v1alpha1.Namer, epr.Namespace, epr.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}
	return test.NewHTTPClient(caCerts), nil
}

func DoRequest(client *http.Client, epr v1alpha1.PackageRegistry, method, path string) ([]byte, error) {
	scheme := "http"
	if epr.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
	}

	url, err := url.Parse(fmt.Sprintf("%s://%s.%s.svc:8080%s", scheme, packageregistry.HTTPServiceName(epr.Name), epr.Namespace, path))
	if err != nil {
		return nil, fmt.Errorf("while parsing URL: %w", err)
	}

	request, err := http.NewRequestWithContext(context.Background(), method, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("while constructing request: %w", err)
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("while making request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			msg:        fmt.Sprintf("fail to request %s, status is %d)", path, resp.StatusCode),
		}
	}
	return io.ReadAll(resp.Body)
}
