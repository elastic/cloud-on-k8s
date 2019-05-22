// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
)

// if `--auto-port-forward` is passed to `go test`, then use a custom
// dialer that sets up port-forwarding to services running within k8s
// (useful when running tests on a dev env instead of as a batch job)

// NewElasticsearchClient returns an ES client for the given stack's ES cluster
func NewElasticsearchClient(es v1alpha1.Elasticsearch, k *K8sHelper) (client.Client, error) {
	password, err := k.GetElasticPassword(es.Name)
	if err != nil {
		return nil, err
	}
	esUser := client.UserAuth{Name: "elastic", Password: password}
	caCert, err := k.GetCACert(es.Name)
	if err != nil {
		return nil, err
	}
	inClusterURL := fmt.Sprintf("https://%s-es.%s.svc.cluster.local:9200", es.Name, es.Namespace)
	var dialer net.Dialer
	if params.AutoPortForward {
		dialer = portforward.NewForwardingDialer()
	}
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	client := client.NewElasticsearchClient(dialer, inClusterURL, esUser, *v, caCert)
	return client, nil
}
