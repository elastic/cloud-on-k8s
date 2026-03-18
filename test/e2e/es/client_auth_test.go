// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// TestClientAuthRequired tests that when client authentication is enabled via the spec field,
// ECK manages self-signed client certificates and the cluster is healthy.
func TestClientAuthRequired(t *testing.T) {
	esName := "test-mtls-required"
	esNamespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(esName).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired()

	k := test.NewK8sClientOrFatal()

	test.StepList{}.
		WithSteps(esBuilder.InitTestSteps(k)).
		WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithSteps(test.StepList{
			{
				Name: "Verify client-certificate-required annotation is set",
				Test: test.Eventually(func() error {
					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: esNamespace,
						Name:      esBuilder.Elasticsearch.Name,
					}, &es); err != nil {
						return err
					}
					if !annotation.HasClientAuthenticationRequired(&es) {
						return fmt.Errorf("annotation %s not found on Elasticsearch resource", annotation.ClientAuthenticationRequiredAnnotation)
					}
					return nil
				}),
			},
			{
				Name: "Verify operator client certificate secret exists",
				Test: test.Eventually(func() error {
					secretName := certificates.OperatorClientCertSecretName(esv1.ESNamer, esBuilder.Elasticsearch.Name)
					var secret corev1.Secret
					return k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: esNamespace,
						Name:      secretName,
					}, &secret)
				}),
			},
			{
				Name: "Verify trust bundle secret exists",
				Test: test.Eventually(func() error {
					secretName := certificates.ClientCertTrustBundleSecretName(esv1.ESNamer, esBuilder.Elasticsearch.Name)
					var secret corev1.Secret
					return k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: esNamespace,
						Name:      secretName,
					}, &secret)
				}),
			},
			{
				Name: "Verify operator client cert is self-signed",
				Test: test.Eventually(func() error {
					secretName := certificates.OperatorClientCertSecretName(esv1.ESNamer, esBuilder.Elasticsearch.Name)
					var secret corev1.Secret
					if err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: esNamespace,
						Name:      secretName,
					}, &secret); err != nil {
						return err
					}

					certs, err := certificates.ParsePEMCerts(secret.Data[certificates.CertFileName])
					if err != nil {
						return err
					}
					if len(certs) == 0 {
						return fmt.Errorf("no certificates found in client cert secret")
					}
					cert := certs[0]
					require.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
					require.Equal(t, cert.Subject.CommonName, cert.Issuer.CommonName)

					// No ca.crt should be present
					require.Empty(t, secret.Data[certificates.CAFileName])
					return nil
				}),
			},
		}).
		WithSteps(esBuilder.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestClientAuthRequiredTransition tests transitioning from no client authentication to required and back to disabled.
// Verifies the cluster remains healthy and client cert secrets are created and cleaned up across transitions.
func TestClientAuthRequiredTransition(t *testing.T) {
	esName := "test-mtls-transition"
	esNamespace := test.Ctx().ManagedNamespace(0)

	// Start with client_authentication disabled (default)
	initialBuilder := elasticsearch.NewBuilder(esName).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	// Phase 1: transition to client_authentication: required
	enabledBuilder := initialBuilder.DeepCopy()
	enabledBuilder.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = true
	enabledBuilder.MutatedFrom = &initialBuilder

	// Phase 2: transition back to disabled
	disabledBuilder := enabledBuilder.DeepCopy()
	disabledBuilder.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false
	disabledBuilder.MutatedFrom = enabledBuilder

	k := test.NewK8sClientOrFatal()

	// Use the builder's actual ES name (includes random suffix from NewBuilder).
	actualESName := initialBuilder.Elasticsearch.Name
	clientCertSecretName := certificates.OperatorClientCertSecretName(esv1.ESNamer, actualESName)
	trustBundleSecretName := certificates.ClientCertTrustBundleSecretName(esv1.ESNamer, actualESName)

	// Helper steps to assert client cert resources exist
	verifyClientCertResourcesExist := test.StepList{
		{
			Name: "Verify client-certificate-required annotation is set",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: esNamespace,
					Name:      actualESName,
				}, &es); err != nil {
					return err
				}
				if !annotation.HasClientAuthenticationRequired(&es) {
					return fmt.Errorf("annotation %s not found on Elasticsearch resource", annotation.ClientAuthenticationRequiredAnnotation)
				}
				return nil
			}),
		},
		{
			Name: "Verify operator client certificate secret exists",
			Test: test.Eventually(func() error {
				var secret corev1.Secret
				return k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: esNamespace,
					Name:      clientCertSecretName,
				}, &secret)
			}),
		},
		{
			Name: "Verify trust bundle secret exists",
			Test: test.Eventually(func() error {
				var secret corev1.Secret
				return k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: esNamespace,
					Name:      trustBundleSecretName,
				}, &secret)
			}),
		},
	}

	// Helper steps to assert client cert resources are removed
	verifyClientCertResourcesRemoved := test.StepList{
		{
			Name: "Verify annotation is removed",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: esNamespace,
					Name:      actualESName,
				}, &es); err != nil {
					return err
				}
				if annotation.HasClientAuthenticationRequired(&es) {
					return fmt.Errorf("annotation %s should be removed", annotation.ClientAuthenticationRequiredAnnotation)
				}
				return nil
			}),
		},
		{
			Name: "Verify operator client certificate secret is deleted",
			Test: test.Eventually(func() error {
				var secret corev1.Secret
				err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: esNamespace,
					Name:      clientCertSecretName,
				}, &secret)
				if apierrors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("operator client certificate secret should be deleted")
			}),
		},
		{
			Name: "Verify trust bundle secret is deleted",
			Test: test.Eventually(func() error {
				var secret corev1.Secret
				err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: esNamespace,
					Name:      trustBundleSecretName,
				}, &secret)
				if apierrors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				return fmt.Errorf("trust bundle secret should be deleted")
			}),
		},
	}

	test.StepList{}.
		// Create with client auth disabled
		WithSteps(initialBuilder.InitTestSteps(k)).
		WithSteps(initialBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initialBuilder, k)).
		WithSteps(verifyClientCertResourcesRemoved).
		// Phase 1: enable client authentication
		// Annotate pods before mutation so CheckExpectedPodsEventuallyReady can verify all pods are rolled.
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(initialBuilder, k)).
		WithSteps(enabledBuilder.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(*enabledBuilder, k)).
		WithSteps(verifyClientCertResourcesExist).
		// Phase 2: disable client authentication
		// Annotate pods before mutation so CheckExpectedPodsEventuallyReady can verify all pods are rolled.
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(*enabledBuilder, k)).
		WithSteps(disabledBuilder.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(*disabledBuilder, k)).
		WithSteps(verifyClientCertResourcesRemoved).
		WithSteps(initialBuilder.DeletionTestSteps(k)).
		RunSequential(t)
}
