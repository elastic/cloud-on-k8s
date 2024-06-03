// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	esuser "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

var PotentialNetworkError = &potentialNetworkError{}

type potentialNetworkError struct {
	err error
}

func (p *potentialNetworkError) Error() string {
	return p.err.Error()
}

func (p *potentialNetworkError) Unwrap() error {
	return p.err
}

// NewElasticsearchClient returns an ES client for the given ES cluster
func NewElasticsearchClient(es esv1.Elasticsearch, k *test.K8sClient) (client.Client, error) {
	password, err := k.GetElasticPassword(k8s.ExtractNamespacedName(&es))
	if err != nil {
		return nil, err
	}
	user := client.BasicAuth{Name: esuser.ElasticUserName, Password: password}
	client, err := NewElasticsearchClientWithUser(es, k, user)
	if err != nil {
		// according to https://github.com/kubernetes/client-go/blob/fb61a7c88cb9f599363919a34b7c54a605455ffc/rest/request.go#L959-L960,
		// client-go requests may return *errors.StatusError or *errors.UnexpectedObjectError, or http client errors.
		// It turns out catching network errors (timeout, connection refused, dns problem) is not trivial
		// (see https://stackoverflow.com/questions/22761562/portable-way-to-detect-different-kinds-of-network-error-in-golang),
		// so here we do the opposite: catch expected apiserver errors, and consider the rest as network errors.
		switch err.(type) { //nolint:errorlint
		case *k8serrors.StatusError, *k8serrors.UnexpectedObjectError:
			// explicit apiserver error, consider as a failure for most checks
			return nil, err
		default:
			// likely a network error, can be acceptable as a transient state depending on the check
			return nil, &potentialNetworkError{err: err}
		}
	}
	return client, nil
}

// NewElasticsearchClientWithUser returns an ES client for the given ES cluster with the given basic auth user.
func NewElasticsearchClientWithUser(es esv1.Elasticsearch, k *test.K8sClient, user client.BasicAuth) (client.Client, error) {
	caCert, err := k.GetHTTPCerts(esv1.ESNamer, es.Namespace, es.Name)
	if err != nil {
		return nil, err
	}
	var dialer net.Dialer
	if test.Ctx().AutoPortForwarding {
		dialer = portforward.NewForwardingDialer()
	}
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	esClient := client.NewElasticsearchClient(
		dialer,
		k8s.ExtractNamespacedName(&es),
		services.NewElasticsearchURLProvider(es, k.Client),
		user,
		v,
		caCert,
		client.Timeout(context.Background(), es),
		true,
	)
	return esClient, nil
}
