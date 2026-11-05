// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package clientauth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// CheckClientCertificatesCountStep returns a Step that verifies the expected number of client
// certificates are trusted by the given server, which runs in the given namespace. Use within
// PostCheckSteps or PostMutationSteps of a WrappedBuilder when composing transition tests with test.RunMutations.
func CheckClientCertificatesCountStep(k *test.K8sClient, namespace, serverName string, expectedCount int) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Verify %d client certificate trust source(s) exist", expectedCount),
		Test: test.Eventually(func() error {
			trustSources, err := listClientCertificateTrustSources(k, namespace, serverName, "", "")
			if err != nil {
				return err
			}
			if len(trustSources) != expectedCount {
				return fmt.Errorf("expected %d client cert trust sources for server %s, got %d", expectedCount, serverName, len(trustSources))
			}
			return nil
		}),
	}
}

// CheckClientCertificateDataStep returns a Step that verifies exactly one client certificate is trusted by the
// given server, which runs in the given namespace, scoped to the given association, and that both the trust
// source and the client certificate secret it was published from contain the expected cert data.
// When associationLabelName and associationLabelValue are empty, no association scoping is applied.
// Use within PostCheckSteps of a WrappedBuilder when composing custom certificate tests with
// test.BeforeAfterSequence.
func CheckClientCertificateDataStep(k *test.K8sClient, namespace, serverName, associationLabelName, associationLabelValue string, expectedCert, expectedKey []byte) test.Step {
	return test.Step{
		Name: "Verify client certificate trust source exists with expected cert data",
		Test: test.Eventually(func() error {
			trustSources, err := listClientCertificateTrustSources(k, namespace, serverName, associationLabelName, associationLabelValue)
			if err != nil {
				return err
			}
			if len(trustSources) != 1 {
				return fmt.Errorf("expected 1 client cert trust source, got %d", len(trustSources))
			}
			trustSource := trustSources[0]
			if string(trustSource.Data[certificates.CertFileName]) != string(expectedCert) {
				return fmt.Errorf("client cert trust source %s has unexpected %s", trustSource.Name, certificates.CertFileName)
			}
			origin, ok := certificates.ClientCertTrustSourceOrigin(trustSource)
			if !ok {
				return fmt.Errorf("client cert trust source %s does not reference its client cert secret", trustSource.Name)
			}
			var secret corev1.Secret
			if err := k.Client.Get(context.Background(), origin, &secret); err != nil {
				return err
			}
			if string(secret.Data[certificates.CertFileName]) != string(expectedCert) {
				return fmt.Errorf("client cert secret %s has unexpected %s", secret.Name, certificates.CertFileName)
			}
			if string(secret.Data[certificates.KeyFileName]) != string(expectedKey) {
				return fmt.Errorf("client cert secret %s has unexpected %s", secret.Name, certificates.KeyFileName)
			}
			return nil
		}),
	}
}

// listClientCertificateTrustSources lists the client certificate trust sources published into the given namespace
// for the given server. When associationLabelName and associationLabelValue are non-empty, results are further
// scoped to trust sources published by that specific association (e.g., APM or Agent).
func listClientCertificateTrustSources(k *test.K8sClient, namespace, serverName, associationLabelName, associationLabelValue string) ([]corev1.Secret, error) {
	var secretList corev1.SecretList
	matchLabels := k8sclient.MatchingLabels{commonv1.TypeLabelName: certificates.ClientCertTrustSourceType}
	if associationLabelName != "" && associationLabelValue != "" {
		matchLabels[associationLabelName] = associationLabelValue
	}
	if err := k.Client.List(context.Background(), &secretList, k8sclient.InNamespace(namespace), matchLabels); err != nil {
		return nil, err
	}

	var trustSources []corev1.Secret
	for _, secret := range secretList.Items {
		if owner := metav1.GetControllerOf(&secret); owner != nil && owner.Name == serverName {
			trustSources = append(trustSources, secret)
		}
	}
	return trustSources, nil
}

// UserCustomCertificateSecretLifecycleSteps returns before/after StepsFuncs that create and delete a user-provided
// client certificate secret for custom certificate tests. The certPEM and keyPEM are the pre-generated
// certificate data to use. Pass the returned StepsFuncs to test.BeforeAfterSequence.
func UserCustomCertificateSecretLifecycleSteps(namespace, secretName string, certPEM, keyPEM []byte) (test.StepsFunc, test.StepsFunc) {
	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Create user-provided client certificate secret",
				Test: func(t *testing.T) {
					t.Helper()
					secret := corev1.Secret{
						Name:      secretName,
						Namespace: namespace,
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
					t.Helper()
					secret := corev1.Secret{
						Name:      secretName,
						Namespace: namespace,
					}
					_ = k.Client.Delete(context.Background(), &secret)
				},
			},
		}
	})

	return before, after
}

// WithNewDataCheck wraps mutated so that, once all the pods recorded by restartChecker are replaced, hasDataSince
// must find documents indexed in es after that point. The pods may belong to another resource than mutated. Wrap
// the result again to add other mutation steps.
func WithNewDataCheck(
	ctx context.Context,
	es types.NamespacedName,
	mutated test.Builder,
	restartChecker *test.PodRestartChecker,
	hasDataSince func(since time.Time) func(esclient.Client) error,
) test.WrappedBuilder {
	return test.WrappedBuilder{
		BuildingThis: mutated,
		PreMutationSteps: func(k *test.K8sClient) test.StepList {
			return restartChecker.RecordUIDs(k)
		},
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return restartChecker.WaitForRestart(k).
				WithSteps(elasticsearch.CheckNewDataSteps(ctx, k, es, hasDataSince))
		},
	}
}
