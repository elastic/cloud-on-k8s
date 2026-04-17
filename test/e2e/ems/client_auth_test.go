// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build ems || e2e

package ems

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	mapscontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	clientauth "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/maps"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, Elastic Maps Server remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-ems-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	emsBuilder := maps.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	// Wrap the ES builder with license setup.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		// 1 client certificate; maps server
		return test.StepList{clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1)}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false
	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.CheckTestSteps(emsBuilder, k).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
					{
						Name: "Verify Maps has no client cert in association conf",
						Test: test.Eventually(func() error {
							var ems emsv1alpha1.ElasticMapsServer
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: namespace,
								Name:      emsBuilder.EMS.Name,
							}, &ems); err != nil {
								return err
							}
							assocConf, err := ems.AssociationConf()
							if err != nil {
								return err
							}
							if assocConf.ClientCertIsConfigured() {
								return fmt.Errorf("Maps association conf should not have a client cert after ES transition, got %s", assocConf.GetClientCertSecretName())
							}
							return nil
						}),
					},
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, emsBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthRequiredCustomCertificate tests that Elastic Maps Server works with a user-provided
// client certificate when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-ems-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	emsBuilder := maps.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:      esBuilder.Elasticsearch.Name,
			Namespace: esBuilder.Elasticsearch.Namespace,
		}).
		WithClientCertificateSecret(userCertSecretName).
		WithNodeCount(1)

	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

	emsWrapped := test.WrappedBuilder{
		BuildingThis: emsBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name, mapscontroller.MapsESAssociationLabelName, emsBuilder.EMS.Name, certPEM, keyPEM)}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), emsWrapped).RunSequential(t)
}
