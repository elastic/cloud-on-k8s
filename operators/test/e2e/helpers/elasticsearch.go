// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"flag"
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/dev/portforward"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
)

// if `--auto-port-forward` is passed to `go test`, then use a custom
// dialer that sets up port-forwarding to services running within k8s
// (useful when running tests on a dev env instead of as a batch job)
var autoPortForward = flag.Bool(
	"auto-port-forward", false,
	"enables automatic port-forwarding (for dev use only as it exposes "+
		"k8s resources on ephemeral ports to localhost)")

// NewElasticsearchClient returns an ES client for the given stack's ES cluster
func NewElasticsearchClient(es v1alpha1.ElasticsearchCluster, k *K8sHelper) (*client.Client, error) {
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
	if *autoPortForward {
		dialer = portforward.NewForwardingDialer()
	}
	client := client.NewElasticsearchClient(dialer, inClusterURL, esUser, caCert)
	return client, nil
}
