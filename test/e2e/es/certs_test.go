// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build es e2e

package es

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestCustomTransportCA(t *testing.T) {
	caSecretName := "my-custom-ca"
	esName := "test-custom-ca"
	esNamespace := test.Ctx().ManagedNamespace(0)

	mkTestSecret := func(cert, key []byte) corev1.Secret {
		return corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: esNamespace,
				Name:      caSecretName,
			},
			Data: map[string][]byte{
				certificates.CertFileName: cert,
				certificates.KeyFileName:  key,
			},
		}
	}

	// Create a multi-node cluster so we have inter-node communication
	initialCluster := elasticsearch.NewBuilder(esName).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	// start with the operator provided transport CA
	withBuiltinCA := test.WrappedBuilder{
		BuildingThis: initialCluster,
		PreDeletionSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Clean up custom CA secret",
					Test: func(t *testing.T) {
						// This is counter-intuitive but deletion steps run on the initial builder
						// let's clean up the CA secret here.
						toDelete := mkTestSecret(nil, nil)
						_ = k.Client.Delete(context.Background(), &toDelete)
					},
					Skip:      nil,
					OnFailure: nil,
				},
			}
		},
	}

	// The initial Cluster builder should result in a healthy cluster as verified by the standard check steps
	// Now modify cluster to use custom transport certs but simulate a user error by populating the secret with
	// garbage and verify this is bubbled up through an event
	withBogusCert := test.WrappedBuilder{
		BuildingThis: initialCluster.
			WithCustomTransportCA(caSecretName).
			WithMutatedFrom(&initialCluster),

		PreMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Create an invalid CA secret",
					Test: test.Eventually(func() error {
						bogusSecret := mkTestSecret([]byte("garbage"), []byte("more garbage"))
						_, err := reconciler.ReconcileSecret(k.Client, bogusSecret, nil)
						return err
					}),
				},
			}

		},
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Invalid CA secret should create events",
					Test: test.Eventually(func() error {
						eventList, err := k.GetEvents(test.EventListOptions(esNamespace, initialCluster.Elasticsearch.Name)...)
						if err != nil {
							return err
						}
						for _, evt := range eventList {
							if evt.Type == corev1.EventTypeWarning &&
								evt.Reason == events.EventReasonValidation &&
								strings.Contains(evt.Message, "can't parse") {
								return nil
							}
						}
						return fmt.Errorf("expected validation event but could not observe it")
					}),
				},
			}
		},
	}

	var ca *certificates.CA

	// A NOOP mutation but set up the CA secret correctly now (using the existing CA generation code in the operator)
	withCustomCert := test.WrappedBuilder{
		BuildingThis: initialCluster.
			WithCustomTransportCA(caSecretName).
			WithMutatedFrom(&initialCluster),

		PreMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Create custom CA secret",
					Test: test.Eventually(func() error {
						var err error
						ca, err = certificates.NewSelfSignedCA(certificates.CABuilderOptions{
							Subject: pkix.Name{
								CommonName:         "eck-e2e-test-custom-ca",
								OrganizationalUnit: []string{"eck-e2e"},
							},
						})
						if err != nil {
							return err
						}

						caSecret := mkTestSecret(
							certificates.EncodePEMCert(ca.Cert.Raw),
							certificates.EncodePEMPrivateKey(*ca.PrivateKey),
						)
						_, err = reconciler.ReconcileSecret(k.Client, caSecret, nil)
						return err
					}),
				},
			}
		},
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Check TLS certs are the expected custom certs",
					Test: test.Eventually(func() error {
						// transport certs are checked as part of stack checks now but let's run this step explicitly once more
						// with the defined CA as a parameter to catch the case where both CA cert in the secret and presented
						// certs on the nodes are not the ones defined by the user
						return elasticsearch.CheckTransportCACertificate(initialCluster.Elasticsearch, ca.Cert)
					}),
				},
			}
		},
	}

	// tests the following sequence:
	// 1. healthy cluster with self-signed operator provided CA
	// 2. reconfigure to use custom certs but simulate user error on certificate setup: cluster to stay healthy
	// 3. reconfigure with correct custom certificates
	// 4. reconfigure back to self-signed operator provided CA
	test.RunMutations(t, []test.Builder{withBuiltinCA}, []test.Builder{withBogusCert, withCustomCert, withBuiltinCA})

}

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
					err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &currentEs)
					require.NoError(t, err)

					b.Elasticsearch = currentEs
					b = b.WithHTTPSAN(podIP)
					require.NoError(t, k.Client.Update(context.Background(), &b.Elasticsearch))
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
	if err := k.Client.Get(context.Background(), key, &secret); err != nil {
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
