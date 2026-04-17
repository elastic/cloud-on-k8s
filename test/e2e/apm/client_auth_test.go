// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	apmcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, APM Server remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-apm-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	apmBuilder := apmserver.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithoutIntegrationCheck().
		WithNodeCount(1)

	// Wrap the ES builder with license setup.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		// 1 client certificate; apm-server
		return test.StepList{clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1)}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false
	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.CheckTestSteps(apmBuilder, k).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
					{
						Name: "Verify APM Server has no client cert in association conf",
						Test: test.Eventually(func() error {
							var apm apmv1.ApmServer
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: namespace,
								Name:      apmBuilder.ApmServer.Name,
							}, &apm); err != nil {
								return err
							}
							for _, assoc := range apm.GetAssociations() {
								conf, err := assoc.AssociationConf()
								if err != nil {
									return err
								}
								if conf != nil && conf.ClientCertIsConfigured() {
									return fmt.Errorf("APM association conf should not have a client cert after ES transition, got %s", conf.GetClientCertSecretName())
								}
							}
							return nil
						}),
					},
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, apmBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthRequiredCustomCertificate tests that APM Server works with a user-provided client certificate
// when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-apm-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	apmBuilder := apmserver.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:      esBuilder.Elasticsearch.Name,
			Namespace: esBuilder.Elasticsearch.Namespace,
		}).
		WithClientCertificateSecret(userCertSecretName).
		WithoutIntegrationCheck().
		WithNodeCount(1)

	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

	apmWrapped := test.WrappedBuilder{
		BuildingThis: apmBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name, apmcontroller.ApmAssociationLabelName, apmBuilder.ApmServer.Name, certPEM, keyPEM)}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), apmWrapped).RunSequential(t)
}
