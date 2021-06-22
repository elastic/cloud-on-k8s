// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"

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

//CheckHTTPConnectivity ensure that we can connect via HTTP to every available Pod.
func (e *esClusterChecks) CheckHTTPConnectivity() test.Step {
	return test.Step{
		Name: "Can connect to all available nodes",
		Test: test.Eventually(func() error {
			es := e.Builder.Elasticsearch
			password, err := e.k.GetElasticPassword(k8s.ExtractNamespacedName(&es))
			if err != nil {
				return err
			}
			user := client.BasicAuth{Name: esuser.ElasticUserName, Password: password}

			caCert, err := e.k.GetHTTPCerts(esv1.ESNamer, es.Namespace, es.Name)
			if err != nil {
				return err
			}
			pods, err := sset.GetActualPodsForCluster(e.k.Client, es)
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
				esClient := client.NewElasticsearchClient(dialer, url, user, v, caCert, client.Timeout(es))
				_, err := esClient.GetClusterInfo(context.Background())
				if err != nil {
					return err
				}
			}
			return nil
		}),
	}
}
