// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build autoops || e2e

package autoops

import (
	"context"
	"fmt"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/autoops"
	clientauth "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestClientAuthRequiredTransition tests that when an Elasticsearch cluster has client authentication enabled,
// the AutoOps agent deployment mounts the client certificate and can connect to ES. It then transitions
// ES to client auth disabled and verifies the client certificate secret is cleaned up and the agent remains healthy.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(version.SupportedAutoOpsAgentBasicVersions.Min) {
		t.Skipf("Skipping test: version %s below minimum %s",
			test.Ctx().ElasticStackVersion, version.SupportedAutoOpsAgentBasicVersions.Min)
	}

	esNamespace := test.Ctx().ManagedNamespace(0)
	policyNamespace := test.Ctx().ManagedNamespace(1)
	mockURL := autoops.CloudConnectedAPIMockURL()

	esBuilder := elasticsearch.NewBuilderWithoutSuffix("es-mtls").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithNamespace(esNamespace).
		WithVersion(test.Ctx().ElasticStackVersion).
		WithLabel("autoops-mtls", "enabled").
		WithClientAuthenticationRequired()

	policyBuilder := autoops.NewBuilder("autoops-mtls").
		WithNamespace(policyNamespace).
		WithResourceSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"autoops-mtls": "enabled"},
		}).
		WithNamespaceSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": esNamespace},
		}).
		WithCloudConnectedAPIURL(mockURL).
		WithAutoOpsOTelURL(mockURL)

	// Wrap the ES builder with license setup and PostCheckSteps to verify client cert secret exists
	// and AutoOps agent pods are ready.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Verify ES has client authentication annotation",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: esNamespace,
						Name:      esBuilder.Elasticsearch.Name,
					}, &es); err != nil {
						return err
					}
					if !annotation.HasClientAuthenticationRequired(&es) {
						return fmt.Errorf("ES should have client authentication required annotation")
					}
					return nil
				}),
			},
			clientauth.CheckClientCertificatesCountStep(k, policyNamespace, esBuilder.Elasticsearch.Name, 1),
		}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			// First wait for the AutoOps policy to be healthy after ES transition, then verify cleanup.
			return test.CheckTestSteps(policyBuilder, k).
				WithSteps(test.StepList{
					{
						Name: "Verify ES no longer has client authentication annotation",
						Test: test.Eventually(func() error {
							var es esv1.Elasticsearch
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: esNamespace,
								Name:      esBuilder.Elasticsearch.Name,
							}, &es); err != nil {
								return err
							}
							if annotation.HasClientAuthenticationRequired(&es) {
								return fmt.Errorf("ES should not have client authentication required annotation after transition")
							}
							return nil
						}),
					},
					clientauth.CheckClientCertificatesCountStep(k, policyNamespace, esBuilder.Elasticsearch.Name, 0),
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, policyBuilder}, []test.Builder{esMutatedWrapped})
}
