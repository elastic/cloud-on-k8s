// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build ent || e2e

package ent

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	entcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
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
		return test.StepList{clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1)}
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

	certPEM, keyPEM := helper.GenerateSelfSignedClientCertPKCS8(t, name)

	entWrapped := test.WrappedBuilder{
		BuildingThis: entBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name, entcontroller.EntESAssociationLabelName, entBuilder.EnterpriseSearch.Name, certPEM, keyPEM)}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), entWrapped).RunSequential(t)
}
