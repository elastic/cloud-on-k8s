// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"crypto/x509"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	esuser "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func CheckHTTPConnectivityWithCA(es esv1.Elasticsearch, k *test.K8sClient, caCert []*x509.Certificate) error {
	password, err := k.GetElasticPassword(k8s.ExtractNamespacedName(&es))
	if err != nil {
		return err
	}
	user := client.BasicAuth{Name: esuser.ElasticUserName, Password: password}

	pods, err := sset.GetActualPodsForCluster(k.Client, es)
	if err != nil {
		return err
	}
	var dialer net.Dialer
	if test.Ctx().AutoPortForwarding {
		dialer = portforward.NewForwardingDialer()
	}
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return err
	}

	for _, p := range reconcile.AvailableElasticsearchNodes(pods) {
		url := services.ElasticsearchPodURL(p)
		esClient := client.NewElasticsearchClient(
			dialer,
			k8s.ExtractNamespacedName(&es),
			client.NewStaticURLProvider(url),
			user,
			v,
			caCert,
			client.Timeout(context.Background(), es),
			true,
		)
		_, err := esClient.GetClusterInfo(context.Background())
		if err != nil {
			return err
		}
	}
	return nil
}
