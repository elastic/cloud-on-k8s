// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build ent || e2e

package ent

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	entcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	clientauth "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, Enterprise Search remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-ent-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	entBuilder := enterprisesearch.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	// Wrap the ES builder with license setup.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		// 1 client certificate; enterprise-search
		return test.StepList{
			clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1),
			{
				// Delete the Enterprise Search client cert secret if its PKCS#8 key's last DER
				// byte falls in the ASCII whitespace range. Enterprise Search would crash on
				// startup with an InvalidKeySpecException due to the Manticore strip() bug
				// (https://github.com/elastic/search-team/issues/15173). Deleting forces the
				// operator to regenerate; ENT's CheckPods step waits for the pod to come up
				// with the new key.
				Name: "Delete Enterprise Search client certificate secret if PKCS#8 key last DER byte is ASCII whitespace",
				Test: test.Eventually(func() error {
					var secretList corev1.SecretList
					if err := k.Client.List(t.Context(), &secretList,
						k8sclient.InNamespace(namespace),
						k8sclient.MatchingLabels{
							labels.ClientCertificateLabelName:       "true",
							entcontroller.EntESAssociationLabelName: entBuilder.EnterpriseSearch.Name,
						},
					); err != nil {
						return err
					}
					if len(secretList.Items) != 1 {
						return fmt.Errorf("expected at most 1 Enterprise Search client cert secret, got %d", len(secretList.Items))
					}
					secret := secretList.Items[0]
					whitespaceByteAtEnd, err := helper.PKCS8KeyEndsWithWhitespaceByte(secret.Data[certificates.KeyFileName])
					if err != nil {
						return err
					}
					if whitespaceByteAtEnd {
						_ = k.Client.Delete(t.Context(), &secret)
						return fmt.Errorf("client cert secret %s has trailing whitespace byte; deleted to force regeneration", secret.Name)
					}
					return nil
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
			return test.CheckTestSteps(entBuilder, k).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
					{
						Name: "Verify Enterprise Search has no client cert in association conf",
						Test: test.Eventually(func() error {
							var ent entv1.EnterpriseSearch
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: namespace,
								Name:      entBuilder.EnterpriseSearch.Name,
							}, &ent); err != nil {
								return err
							}
							assocConf, err := ent.AssociationConf()
							if err != nil {
								return err
							}
							if assocConf.ClientCertIsConfigured() {
								return fmt.Errorf("Enterprise Search association conf should not have a client cert after ES transition, got %s", assocConf.GetClientCertSecretName())
							}
							return nil
						}),
					},
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, entBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthRequiredCustomCertificate tests that Enterprise Search works with a user-provided
// client certificate when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-ent-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	entBuilder := enterprisesearch.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:      esBuilder.Elasticsearch.Name,
			Namespace: esBuilder.Elasticsearch.Namespace,
		}).
		WithClientCertificateSecret(userCertSecretName).
		WithNodeCount(1)

	var certPEM, keyPEM []byte
	test.Eventually(func() error {
		certPEM, keyPEM = helper.GenerateSelfSignedClientCertPKCS8(t, name)
		whitespaceByteAtEnd, err := helper.PKCS8KeyEndsWithWhitespaceByte(keyPEM)
		if err != nil {
			return err
		}
		if whitespaceByteAtEnd {
			// Regenerate the Enterprise Search client user cert secret if its PKCS#8 key's last DER
			// byte falls in the ASCII whitespace range. Enterprise Search would crash on
			// startup with an InvalidKeySpecException due to the Manticore strip() bug
			// (https://github.com/elastic/search-team/issues/15173).
			return fmt.Errorf("regenerating client cert: PKCS#8 key ends with ASCII whitespace byte")
		}
		return nil
	})(t)

	entWrapped := test.WrappedBuilder{
		BuildingThis: entBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name, entcontroller.EntESAssociationLabelName, entBuilder.EnterpriseSearch.Name, certPEM, keyPEM)}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), entWrapped).RunSequential(t)
}
