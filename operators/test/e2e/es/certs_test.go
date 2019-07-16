// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestUpdateHTTPCertSAN(t *testing.T) {
	name := "test-http-cert-san"
	b := elasticsearch.NewBuilder(name).
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithHTTPLoadBalancer()

	var caCert []byte
	var publicIP string

	steps := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Retrieve ES certificate",
				Test: func(t *testing.T) {
					var err error
					caCert, err = getCert(k, test.Namespace, name)
					require.NoError(t, err)
				},
			},
			{
				Name: "Retrieve ES public IP",
				Test: test.Eventually(func() error {
					var err error
					publicIP, err = getPublicIP(k, test.Namespace, name)
					return err
				}),
			},
			{
				Name: "Check ES is not reachable with cert verification",
				Test: func(t *testing.T) {
					_, err := requestESWithCA(publicIP, caCert)
					require.Error(t, err)
					require.Contains(t, err.Error(), "x509: cannot validate certificate")
				},
			},
			{
				Name: "Add load balancer IP to the SAN",
				Test: func(t *testing.T) {
					var currentEs v1alpha1.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &currentEs)
					require.NoError(t, err)

					b.Elasticsearch = currentEs
					b = b.WithHTTPSAN(publicIP)
					require.NoError(t, k.Client.Update(&b.Elasticsearch))
				},
			},
			{
				Name: "Check ES is reachable with cert verification",
				Test: test.Eventually(func() error {
					status, err := requestESWithCA(publicIP, caCert)
					if err != nil {
						return err
					}
					fmt.Println("s:", status)
					if status != 401 {
						return fmt.Errorf("invalid status code to reach ES: %d", status)
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(nil, steps, b).RunSequential(t)
}

func getCert(k *test.K8sClient, ns string, esName string) ([]byte, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: ns,
		Name:      certificates.PublicSecretName(esname.ESNamer, esName, certificates.HTTPCAType),
	}
	if err := k.Client.Get(key, &secret); err != nil {
		return nil, err
	}
	certBytes, exists := secret.Data[certificates.CertFileName]
	if !exists || len(certBytes) == 0 {
		return nil, fmt.Errorf("no value found for secret %s", certificates.CertFileName)
	}

	return certBytes, nil
}

func getPublicIP(k *test.K8sClient, ns string, esName string) (string, error) {
	var svc corev1.Service
	key := types.NamespacedName{
		Namespace: ns,
		Name:      esname.HTTPService(esName),
	}
	if err := k.Client.Get(key, &svc); err != nil {
		return "", err
	}

	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			return ing.IP, nil
		}
	}

	return "", errors.New("no external IP found")
}

func requestESWithCA(ip string, caCert []byte) (int, error) {
	url := fmt.Sprintf("https://%s:9200", ip)

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	client := &http.Client{}
	if caCert != nil {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}

	return resp.StatusCode, nil
}
