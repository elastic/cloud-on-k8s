// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build beat || e2e

package beat

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
)

// TestClientAuthRequiredTransition tests that when Elasticsearch transitions from client authentication
// required to disabled, Beat remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-beat-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	fileBeatConfig := E2EFilebeatConfig
	if !SupportsFingerprintIdentity(version.MustParse(test.Ctx().ElasticStackVersion)) {
		fileBeatConfig = E2EFilebeatConfigPRE810
	}

	testPodBuilder := beat.NewPodBuilder(name)

	beatBuilder := beat.NewBuilder(name).
		WithType("filebeat").
		WithRoles(beat.AutodiscoverClusterRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithElasticsearchRef(esBuilder.Ref()).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEventFromPod(testPodBuilder.Pod.Name),
		)
	beatBuilder = beat.ApplyYamls(t, beatBuilder, fileBeatConfig, E2EFilebeatPodTemplate)

	// Wrap the ES builder with license setup and PostCheckSteps to verify client cert secret exists.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{
			clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1),
		}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			// First wait for all Beat pods to be ready after ES transition before checking cleanup.
			return test.CheckTestSteps(beatBuilder, k).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
					{
						Name: "Verify Beat has no client cert in association conf",
						Test: test.Eventually(func() error {
							var b beatv1beta1.Beat
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: namespace,
								Name:      beatBuilder.Beat.Name,
							}, &b); err != nil {
								return err
							}
							assocConf, err := b.EsAssociation().AssociationConf()
							if err != nil {
								return err
							}
							if assocConf.ClientCertIsConfigured() {
								return fmt.Errorf("Beat association conf should not have a client cert secret after ES transition, got %s", assocConf.GetClientCertSecretName())
							}
							return nil
						}),
					},
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, beatBuilder, testPodBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthRequiredCustomCertificate tests that Beat works with a user-provided client certificate
// when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-beat-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	fileBeatConfig := E2EFilebeatConfig
	if !SupportsFingerprintIdentity(version.MustParse(test.Ctx().ElasticStackVersion)) {
		fileBeatConfig = E2EFilebeatConfigPRE810
	}

	testPodBuilder := beat.NewPodBuilder(name)

	// Generate cert upfront so we can verify the data in the managed secret.
	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

	beatBuilder := beat.NewBuilder(name).
		WithType("filebeat").
		WithRoles(beat.AutodiscoverClusterRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:      esBuilder.Elasticsearch.Name,
			Namespace: esBuilder.Elasticsearch.Namespace,
		}).
		WithClientCertificateSecret(userCertSecretName).
		WithESValidations(
			beat.HasEventFromBeat(filebeat.Type),
			beat.HasEventFromPod(testPodBuilder.Pod.Name),
		)
	beatBuilder = beat.ApplyYamls(t, beatBuilder, fileBeatConfig, E2EFilebeatPodTemplate)

	// Wrap the Beat builder to add post-check verification steps.
	beatWrapped := test.WrappedBuilder{
		BuildingThis: beatBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Verify Beat association conf has client cert configured",
					Test: test.Eventually(func() error {
						var b beatv1beta1.Beat
						if err := k.Client.Get(context.Background(), types.NamespacedName{
							Namespace: namespace,
							Name:      beatBuilder.Beat.Name,
						}, &b); err != nil {
							return err
						}
						assocConf, err := b.EsAssociation().AssociationConf()
						if err != nil {
							return err
						}
						if !assocConf.ClientCertIsConfigured() {
							return fmt.Errorf("Beat association conf should have a client cert secret configured")
						}
						return nil
					}),
				},
				clientauth.CheckClientCertificateDataStep(
					k, namespace, esBuilder.Elasticsearch.Name,
					controller.BeatAssociationLabelName, beatBuilder.Beat.Name,
					certPEM, keyPEM,
				),
			}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), beatWrapped, testPodBuilder).RunSequential(t)
}
