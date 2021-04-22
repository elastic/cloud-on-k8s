// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package maps

import (
	"context"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/maps"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type APIError struct {
	StatusCode int
	msg        string
}

func (e *APIError) Error() string {
	return e.msg
}

// TODO refactor identical to Kibana client
func NewMapsClient(ems v1alpha1.ElasticMapsServer, k *test.K8sClient) (*http.Client, error) {
	var caCerts []*x509.Certificate
	if ems.Spec.HTTP.TLS.Enabled() {
		crts, err := k.GetHTTPCerts(maps.EMSNamer, ems.Namespace, ems.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}
	return test.NewHTTPClient(caCerts), nil
}

func DoRequest(client *http.Client, ems v1alpha1.ElasticMapsServer, method, path string) ([]byte, error) {
	scheme := "http"
	if ems.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
	}

	url, err := url.Parse(fmt.Sprintf("%s://%s.%s.svc:8080%s", scheme, maps.HTTPService(ems.Name), ems.Namespace, path))
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

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			msg:        fmt.Sprintf("fail to request %s, status is %d)", path, resp.StatusCode),
		}
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}
