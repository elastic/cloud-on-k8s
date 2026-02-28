// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, Kibana remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	name := "test-kb-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	k := test.NewK8sClientOrFatal()

	// Phase 1: create ES with client auth + Kibana, verify healthy and client cert secret exists
	steps := test.StepList{}.
		WithSteps(esBuilder.InitTestSteps(k)).
		WithSteps(kbBuilder.InitTestSteps(k)).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(kbBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithSteps(test.CheckTestSteps(kbBuilder, k)).
		WithSteps(test.StepList{
			{
				Name: "Verify Kibana client certificate secret exists",
				Test: test.Eventually(func() error {
					return verifyClientCertSecretCount(k, namespace, esBuilder.Elasticsearch.Name, 1)
				}),
			},
		})

	// Phase 2: transition ES to client auth disabled
	esMutated := esBuilder.DeepCopy()
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false
	esMutated.MutatedFrom = &esBuilder

	steps = steps.
		WithSteps(esMutated.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(*esMutated, k)).
		// Wait for all Kibana pods to be ready after ES transition before checking cleanup.
		WithSteps(test.CheckTestSteps(kbBuilder, k)).
		WithSteps(test.StepList{
			{
				Name: "Verify Kibana client certificate secret is deleted",
				Test: test.Eventually(func() error {
					return verifyClientCertSecretCount(k, namespace, esBuilder.Elasticsearch.Name, 0)
				}),
			},
			{
				Name: "Verify Kibana has no client cert in association conf",
				Test: test.Eventually(func() error {
					var kb kbv1.Kibana
					if err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: namespace,
						Name:      kbBuilder.Kibana.Name,
					}, &kb); err != nil {
						return err
					}
					assocConf, err := kb.EsAssociation().AssociationConf()
					if err != nil {
						return err
					}
					if assocConf.ClientCertIsConfigured() {
						return fmt.Errorf("Kibana association conf should not have a client cert secret after ES transition, got %s", assocConf.GetClientCertSecretName())
					}
					return nil
				}),
			},
		}).
		WithSteps(kbBuilder.DeletionTestSteps(k)).
		WithSteps(esBuilder.DeletionTestSteps(k))

	steps.RunSequential(t)
}

// TestClientAuthRequiredCustomCertificate tests that Kibana works with a user-provided client certificate
// when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	name := "test-kb-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:                        esBuilder.Elasticsearch.Name,
			Namespace:                   esBuilder.Elasticsearch.Namespace,
			ClientCertificateSecretName: userCertSecretName,
		}).
		WithNodeCount(1)

	k := test.NewK8sClientOrFatal()

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create user-provided client certificate secret",
				Test: func(t *testing.T) {
					certPEM, keyPEM := generateSelfSignedClientCert(t, name)
					secret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      userCertSecretName,
							Namespace: namespace,
						},
						Data: map[string][]byte{
							certificates.CertFileName: certPEM,
							certificates.KeyFileName:  keyPEM,
						},
					}
					require.NoError(t, k.Client.Create(context.Background(), &secret))
				},
			},
		}
	})

	after := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete user-provided client certificate secret",
				Test: func(t *testing.T) {
					secret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      userCertSecretName,
							Namespace: namespace,
						},
					}
					_ = k.Client.Delete(context.Background(), &secret)
				},
			},
		}
	})

	steps := test.StepList{}
	steps = steps.WithSteps(before(k))
	steps = steps.
		WithSteps(esBuilder.InitTestSteps(k)).
		WithSteps(kbBuilder.InitTestSteps(k)).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(kbBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithSteps(test.CheckTestSteps(kbBuilder, k)).
		WithSteps(test.StepList{
			{
				Name: "Verify Kibana association conf has client cert configured",
				Test: test.Eventually(func() error {
					var kb kbv1.Kibana
					if err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: namespace,
						Name:      kbBuilder.Kibana.Name,
					}, &kb); err != nil {
						return err
					}
					assocConf, err := kb.EsAssociation().AssociationConf()
					if err != nil {
						return err
					}
					if !assocConf.ClientCertIsConfigured() {
						return fmt.Errorf("Kibana association conf should have a client cert secret configured")
					}
					return nil
				}),
			},
			{
				Name: "Verify managed client certificate secret exists with user cert data",
				Test: test.Eventually(func() error {
					secrets, err := listClientCertSecrets(k, namespace, esBuilder.Elasticsearch.Name)
					if err != nil {
						return err
					}
					if len(secrets) != 1 {
						return fmt.Errorf("expected 1 client cert secret, got %d", len(secrets))
					}
					secret := secrets[0]
					if _, ok := secret.Data[certificates.CertFileName]; !ok {
						return fmt.Errorf("managed client cert secret is missing %s", certificates.CertFileName)
					}
					if _, ok := secret.Data[certificates.KeyFileName]; !ok {
						return fmt.Errorf("managed client cert secret is missing %s", certificates.KeyFileName)
					}
					return nil
				}),
			},
		}).
		WithSteps(kbBuilder.DeletionTestSteps(k)).
		WithSteps(esBuilder.DeletionTestSteps(k))
	steps = steps.WithSteps(after(k))

	steps.RunSequential(t)
}

// verifyClientCertSecretCount verifies the number of client certificate secrets
// associated with the given ES resource in the given namespace.
func verifyClientCertSecretCount(k *test.K8sClient, namespace, esName string, expectedCount int) error {
	secrets, err := listClientCertSecrets(k, namespace, esName)
	if err != nil {
		return err
	}
	if len(secrets) != expectedCount {
		return fmt.Errorf("expected %d client cert secrets for ES %s, got %d", expectedCount, esName, len(secrets))
	}
	return nil
}

// listClientCertSecrets lists secrets with the client certificate label that are soft-owned by the given ES resource.
func listClientCertSecrets(k *test.K8sClient, namespace, esName string) ([]corev1.Secret, error) {
	var secretList corev1.SecretList
	matchLabels := k8sclient.MatchingLabels{
		labels.ClientCertificateLabelName: "true",
	}
	if err := k.Client.List(context.Background(), &secretList, k8sclient.InNamespace(namespace), matchLabels); err != nil {
		return nil, err
	}
	// Filter to secrets soft-owned by the given ES name.
	var filtered []corev1.Secret
	for _, s := range secretList.Items {
		if s.Labels[reconciler.SoftOwnerNameLabel] == esName {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// generateSelfSignedClientCert generates a self-signed client certificate and returns PEM-encoded cert and key.
func generateSelfSignedClientCert(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	require.NoError(t, err)

	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         cn,
			OrganizationalUnit: []string{"eck-e2e-test"},
		},
		NotBefore:          time.Now().Add(-10 * time.Minute),
		NotAfter:           time.Now().Add(24 * time.Hour),
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	certDER, err := x509.CreateCertificate(cryptorand.Reader, &template, &template, privateKey.Public(), privateKey)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}
