// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// NewElasticsearchClient returns an ES client for the given ES cluster
func NewElasticsearchClient(es esv1.Elasticsearch, k *test.K8sClient) (client.Client, error) {
	password, err := k.GetElasticPassword(k8s.ExtractNamespacedName(&es))
	if err != nil {
		return nil, err
	}
	user := client.BasicAuth{Name: esuser.ElasticUserName, Password: password}
	return NewElasticsearchClientWithUser(es, k, user)
}

// NewElasticsearchClientWithUser returns an ES client for the given ES cluster with the given basic auth user.
func NewElasticsearchClientWithUser(es esv1.Elasticsearch, k *test.K8sClient, user client.BasicAuth) (client.Client, error) {
	caCert, err := k.GetHTTPCerts(esv1.ESNamer, es.Namespace, es.Name)
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
	esClient := client.NewElasticsearchClient(dialer, inClusterURL, user, *v, caCert)
	return esClient, nil
}
