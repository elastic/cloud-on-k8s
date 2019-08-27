// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// NewElasticsearchClient returns an ES client for the given ES cluster
func NewElasticsearchClient(es v1alpha1.Elasticsearch, k *test.K8sClient) (client.Client, error) {
	password, err := k.GetElasticPassword(es.Namespace, es.Name)
	if err != nil {
		return nil, err
	}
	esUser := client.UserAuth{Name: "elastic", Password: password}

	caCert, err := k.GetHTTPCerts(name.ESNamer, es.Namespace, es.Name)
	if err != nil {
		return nil, err
	}
	pods, err := sset.GetActualPodsForCluster(k.Client, es)
	if err != nil {
		return nil, err
	}
	inClusterURL := services.ElasticsearchURL(es, reconcile.AvailableElasticsearchNodes(pods))
	var dialer net.Dialer
	if test.Ctx().AutoPortForwarding {
		dialer = portforward.NewForwardingDialer()
	}
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	esClient := client.NewElasticsearchClient(dialer, inClusterURL, esUser, *v, caCert)
	return esClient, nil
}
