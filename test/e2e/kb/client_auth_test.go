// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	clientauth "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
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
		return test.StepList{clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1)}
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
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
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

// TestPKIAuthenticationModeTransition verifies the PKI authentication mode lifecycle for Kibana: Kibana
// authenticates to Elasticsearch with its client certificate only, and switching it back to credentials cleans up
// the eck_managed_pki_ role mapping.
func TestPKIAuthenticationModeTransition(t *testing.T) {
	clientauth.SkipIfPKINotSupported(t)

	name := "test-kb-pki-trans"
	esBuilder := clientauth.NewPKIElasticsearchBuilder(name)

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)
	kbBuilder.Kibana.Spec.ElasticsearchRef.AuthenticationMode = commonv1.AuthenticationModePKI

	kbMutated := kbBuilder.DeepCopy().WithMutatedFrom(&kbBuilder)
	kbMutated.Kibana.Spec.ElasticsearchRef.AuthenticationMode = commonv1.AuthenticationModeCredentials

	clientauth.RunPKIAuthenticationModeTransition(t, esBuilder, kbBuilder, kbMutated)
}

// TestPKIEnablementTransition verifies that Kibana moves to the PKI authentication mode on a running cluster that
// requires client authentication but has no ECK-managed PKI realm yet. Kibana is switched to PKI before the realm is
// enabled, so its role mapping request exists while the nodes restart one by one to load the realm. Kibana must keep
// its credentials until the realm runs on all the nodes, then authenticate with its client certificate only.
func TestPKIEnablementTransition(t *testing.T) {
	clientauth.SkipIfPKINotSupported(t)

	name := "test-kb-pki-enable"
	namespace := test.Ctx().ManagedNamespace(0)

	// Elasticsearch requires client authentication, but the PKI realm is not enabled yet.
	esBuilder := clientauth.NewPKIElasticsearchBuilder(name)
	esBuilder.Elasticsearch.Spec.Auth.PKI.Enabled = false

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.Auth.PKI.Enabled = true

	kbMutated := kbBuilder.DeepCopy().WithMutatedFrom(&kbBuilder)
	kbMutated.Kibana.Spec.ElasticsearchRef.AuthenticationMode = commonv1.AuthenticationModePKI

	// The realm is a static setting, so a Ready Elasticsearch pod that was not restarted serves requests without it.
	// While such a pod exists, Kibana must keep its credentials, whatever the progress of the rolling restart.
	esRestartChecker := test.NewPodRestartChecker("Elasticsearch", test.ESPodListOptions(namespace, esBuilder.Elasticsearch.Name)...)
	// the watcher outlives the step starting it, whose context is canceled when the step ends
	ctx := t.Context()
	var credentialsLost []error
	kbCredentialsWatcher := test.NewWatcher("watch that Kibana keeps its credentials while Elasticsearch pods without the PKI realm are Ready",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) { //nolint:thelper
			// Kibana is read before the pods: a pod that was not restarted and is still Ready afterwards was already
			// running without the realm when Kibana switched.
			var kb kbv1.Kibana
			if err := k.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: kbBuilder.Kibana.Name}, &kb); err != nil {
				t.Logf("failed to get Kibana: %v", err)
				return
			}
			assocConf, err := kb.EsAssociation().AssociationConf()
			if err != nil {
				t.Logf("failed to get the Kibana association conf: %v", err)
				return
			}
			if !assocConf.NoAuthRequired() {
				return
			}
			notRestarted, err := esRestartChecker.ReadyPodsNotRestarted(ctx, k)
			if err != nil {
				t.Logf("failed to list Elasticsearch pods: %v", err)
				return
			}
			if len(notRestarted) > 0 {
				credentialsLost = append(credentialsLost, fmt.Errorf("authSecretName is %q while Elasticsearch pods %v are Ready without the PKI realm", commonv1.NoAuthRequiredValue, notRestarted))
			}
		},
		func(_ *test.K8sClient, t *testing.T) { //nolint:thelper
			require.Empty(t, credentialsLost)
		})

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PreMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Switch Kibana to the PKI authentication mode before the PKI realm is enabled",
					Test: test.Eventually(func() error {
						var kb kbv1.Kibana
						if err := k.Client.Get(t.Context(), types.NamespacedName{
							Namespace: namespace,
							Name:      kbBuilder.Kibana.Name,
						}, &kb); err != nil {
							return err
						}
						kb.Spec.ElasticsearchRef.AuthenticationMode = commonv1.AuthenticationModePKI
						return k.Client.Update(t.Context(), &kb)
					}),
				},
			}.
				WithSteps(esRestartChecker.RecordUIDs(k)).
				WithStep(kbCredentialsWatcher.StartStep(k))
		},
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			// Kibana was switched to PKI before the mutation, so it is checked directly rather than mutated again:
			// the Kibana mutation steps would annotate its pod, possibly already in PKI mode, with the hash of the
			// credentials spec, and reapplying the same spec would never replace it.
			return test.StepList{
				{
					Name: "Kibana should switch to the PKI authentication mode once the realm runs on all the nodes",
					Test: test.Eventually(func() error {
						var kb kbv1.Kibana
						if err := k.Client.Get(t.Context(), types.NamespacedName{Namespace: namespace, Name: kbBuilder.Kibana.Name}, &kb); err != nil {
							return err
						}
						assocConf, err := kb.EsAssociation().AssociationConf()
						if err != nil {
							return err
						}
						if !assocConf.NoAuthRequired() {
							return fmt.Errorf("authSecretName is %q, expected %q", assocConf.AuthSecretName, commonv1.NoAuthRequiredValue)
						}
						return nil
					}),
				},
				kbCredentialsWatcher.StopStep(k),
				clientauth.CheckFileSettingsPKIEntriesStep(t.Context(), k, namespace, esBuilder.Elasticsearch.Name, true),
			}.
				WithSteps(kbMutated.CheckK8sTestSteps(k)).
				WithSteps(kbMutated.CheckStackTestSteps(k))
		},
	}

	test.RunMutations(t,
		[]test.Builder{test.LicenseTestBuilder(esBuilder), kbBuilder},
		[]test.Builder{esMutatedWrapped},
	)
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
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name:      esBuilder.Elasticsearch.Name,
			Namespace: esBuilder.Elasticsearch.Namespace,
		}).
		WithClientCertificateSecret(userCertSecretName).
		WithNodeCount(1)

	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

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
				clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name,
					controller.KibanaAssociationLabelName, kbBuilder.Kibana.Name, certPEM, keyPEM),
			}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), kbWrapped).RunSequential(t)
}
