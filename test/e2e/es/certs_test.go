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
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestUpdateHTTPCertSAN(t *testing.T) {
	b := elasticsearch.NewBuilder("test-http-cert-san").
		WithESMasterNodes(1, elasticsearch.DefaultResources)

	var caCert []byte
	var podIP string

	steps := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Retrieve ES certificate",
				Test: func(t *testing.T) {
					var err error
					caCert, err = getCert(k, b.Elasticsearch.Namespace, b.Elasticsearch.Name)
					require.NoError(t, err)
				},
			},
			{
				Name: "Retrieve a POD IP",
				Test: test.Eventually(func() error {
					var err error
					podIP, err = getPodIP(k, b.Elasticsearch.Namespace, b.Elasticsearch.Name)
					return err
				}),
			},
			{
				Name: "Check ES is not reachable with cert verification",
				Test: func(t *testing.T) {
					_, err := requestESWithCA(podIP, caCert)
					require.Error(t, err)
					require.Contains(t, err.Error(), "x509: cannot validate certificate")
				},
			},
			{
				Name: "Add load balancer IP to the SAN",
				Test: func(t *testing.T) {
					var currentEs esv1.Elasticsearch
					err := k.Client.Get(k8s.ExtractNamespacedName(&b.Elasticsearch), &currentEs)
					require.NoError(t, err)

					b.Elasticsearch = currentEs
					b = b.WithHTTPSAN(podIP)
					require.NoError(t, k.Client.Update(&b.Elasticsearch))
				},
			},
			{
				Name: "Check ES is reachable with cert verification",
				Test: test.Eventually(func() error {
					status, err := requestESWithCA(podIP, caCert)
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
		Name:      certificates.PublicCertsSecretName(esv1.ESNamer, esName),
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

func getPodIP(k *test.K8sClient, ns string, esName string) (string, error) {

	pods, err := k.GetPods(test.ESPodListOptions(ns, esName)...)
	if err != nil {
		return "", err
	}
	for _, pod := range pods {
		if len(pod.Status.PodIP) > 0 {
			return pod.Status.PodIP, nil
		}
	}

	return "", errors.New("no external IP found")
}

func requestESWithCA(ip string, caCert []byte) (int, error) {
	url := fmt.Sprintf("https://%s:9200", ip)

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	transport := http.Transport{}
	if test.Ctx().AutoPortForwarding {
		transport.DialContext = portforward.NewForwardingDialer().DialContext
	}
	if caCert != nil {
		transport.TLSClientConfig = &tls.Config{
			RootCAs: caCertPool,
		}
	}

	client := http.Client{
		Timeout:   60 * time.Second,
		Transport: &transport,
	}

	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}
