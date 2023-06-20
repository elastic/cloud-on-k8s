// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	ls "github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

// TODO refactor identical to Kibana client
func NewLogstashClient(logstash v1alpha1.Logstash, _ *test.K8sClient) (*http.Client, error) {
	var caCerts []*x509.Certificate
	// TODO: Integrate with TLS on metrics API
	// if ems.Spec.HTTP.TLS.Enabled() {
	//	crts, err := k.GetHTTPCerts(maps.EMSNamer, ems.Namespace, ems.Name)
	//	if err != nil {
	//		return nil, err
	//	}
	//	caCerts = crts
	//}
	return test.NewHTTPClient(caCerts), nil
}

func DoRequest(client *http.Client, logstash v1alpha1.Logstash, method, path string) ([]byte, error) {
	var scheme = "http"
	var port = network.HTTPPort
	for _, service := range logstash.Spec.Services {
		if service.Name == ls.LogstashAPIServiceName && len(service.Service.Spec.Ports) > 0 {
			port = int(service.Service.Spec.Ports[0].Port)
		}
	}

	url, err := url.Parse(fmt.Sprintf("%s://%s.%s.svc:%d%s", scheme, v1alpha1.APIServiceName(logstash.Name), logstash.Namespace, port, path))

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
		return nil, fmt.Errorf("fail to request %s, status is %d)", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
