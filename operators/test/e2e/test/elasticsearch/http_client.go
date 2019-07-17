// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
)

// NewElasticsearchClient returns an ES client for the given ES cluster
func NewElasticsearchClient(es v1alpha1.Elasticsearch, k *test.K8sClient) (client.Client, error) {
	password, err := k.GetElasticPassword(es.Name)
	if err != nil {
		return nil, err
	}
	esUser := client.UserAuth{Name: "elastic", Password: password}

	caCert, err := k.GetHTTPCerts(name.ESNamer, es.Name)
	if err != nil {
		return nil, err
	}
	inClusterURL := fmt.Sprintf("https://%s:9200", name.HTTPService(es.Name))
	var dialer net.Dialer
	if test.AutoPortForward {
		dialer = portforward.NewForwardingDialer()
	}
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	esClient := client.NewElasticsearchClient(dialer, inClusterURL, esUser, *v, caCert)
	return esClient, nil
}
