// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	"fmt"
	"testing"

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
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, Kibana remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-kb-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	// Wrap the ES builder with license setup and PostCheckSteps to verify client cert secret exists.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Verify Kibana client certificate secret exists",
				Test: test.Eventually(func() error {
					return verifyClientCertSecretCount(k, namespace, esBuilder.Elasticsearch.Name, 1)
				}),
			},
		}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			// First wait for all Kibana pods to be ready after ES transition before checking cleanup.
			return test.CheckTestSteps(kbBuilder, k).
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
								return fmt.Errorf("kibana association conf should not have a client cert secret after ES transition, got %s", assocConf.GetClientCertSecretName())
							}
							return nil
						}),
					},
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, kbBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthRequiredCustomCertificate tests that Kibana works with a user-provided client certificate
// when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-kb-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:      esBuilder.Elasticsearch.Name,
			Namespace: esBuilder.Elasticsearch.Namespace,
		}).
		WithClientCertificateSecret(userCertSecretName).
		WithNodeCount(1)

	// Wrap the Kibana builder to add post-check verification steps.
	kbWrapped := test.WrappedBuilder{
		BuildingThis: kbBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
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
			}
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create user-provided client certificate secret",
				Test: func(t *testing.T) {
					certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)
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

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), kbWrapped).RunSequential(t)
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
