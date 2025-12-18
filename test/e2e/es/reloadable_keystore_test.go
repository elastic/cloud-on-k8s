// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorejob"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// TestReloadableKeystore tests the reloadable keystore feature for Elasticsearch 9.3+.
// This feature allows updating secure settings without triggering pod restarts.
func TestReloadableKeystore(t *testing.T) {
	// Skip if version is below 9.3.0
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	if stackVersion.LT(keystorejob.MinVersion) {
		t.Skipf("Skipping reloadable keystore test: version %s is below minimum %s", stackVersion, keystorejob.MinVersion)
	}

	k := test.NewK8sClientOrFatal()

	// Secure settings secrets
	const securePasswordSettingKey = "xpack.notification.email.account.foo.smtp.secure_password"
	const secureBarUserSettingKey = "xpack.notification.jira.account.bar.secure_user"

	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reloadable-keystore-secrets",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			securePasswordSettingKey: []byte("initial_password"),
		},
	}

	// Set up a cluster with secure settings
	b := elasticsearch.NewBuilder("test-reload-ks").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettings.Name)

	// Track pod UIDs to verify no restarts
	var initialPodUIDs map[string]string

	test.StepList{}.
		// Create secure settings secret
		WithStep(test.Step{
			Name: "Create secure settings secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSettings)
			}),
		}).

		// Create the cluster
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithSteps(test.StepList{
			// Verify initial secure settings are in the keystore
			elasticsearch.CheckESKeystoreEntries(k, b, []string{
				securePasswordSettingKey,
			}),

			// Record initial pod UIDs
			test.Step{
				Name: "Record initial pod UIDs",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
					if err != nil {
						return err
					}
					initialPodUIDs = make(map[string]string)
					for _, pod := range pods {
						initialPodUIDs[pod.Name] = string(pod.UID)
					}
					if len(initialPodUIDs) == 0 {
						return fmt.Errorf("no pods found")
					}
					return nil
				}),
			},

			// Verify keystore secret exists with expected annotations
			test.Step{
				Name: "Verify keystore secret has expected annotations",
				Test: test.Eventually(func() error {
					var keystoreSecret corev1.Secret
					secretName := esv1.KeystoreSecretName(b.Elasticsearch.Name)
					nsn := types.NamespacedName{Namespace: b.Elasticsearch.Namespace, Name: secretName}
					if err := k.Client.Get(context.Background(), nsn, &keystoreSecret); err != nil {
						return err
					}

					// Check for expected annotations
					if _, ok := keystoreSecret.Annotations[esv1.KeystoreHashAnnotation]; !ok {
						return fmt.Errorf("keystore secret missing %s annotation", esv1.KeystoreHashAnnotation)
					}
					if _, ok := keystoreSecret.Annotations[esv1.KeystoreDigestAnnotation]; !ok {
						return fmt.Errorf("keystore secret missing %s annotation", esv1.KeystoreDigestAnnotation)
					}
					return nil
				}),
			},

			// Update the secure settings secret
			test.Step{
				Name: "Update secure settings secret",
				Test: test.Eventually(func() error {
					// Add a new key
					secureSettings.Data[secureBarUserSettingKey] = []byte("bar_user_value")
					return k.Client.Update(context.Background(), &secureSettings)
				}),
			},

			// Keystore should be updated with both keys
			elasticsearch.CheckESKeystoreEntries(k, b, []string{
				securePasswordSettingKey,
				secureBarUserSettingKey,
			}),

			// Verify no pod restarts occurred (same UIDs)
			test.Step{
				Name: "Verify pods were NOT restarted after keystore update",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Namespace, b.Elasticsearch.Name)...)
					if err != nil {
						return err
					}
					for _, pod := range pods {
						initialUID, exists := initialPodUIDs[pod.Name]
						if !exists {
							// New pod name - this could happen if scaling occurred, but for this test we expect same pods
							return fmt.Errorf("unexpected new pod %s appeared", pod.Name)
						}
						if string(pod.UID) != initialUID {
							return fmt.Errorf("pod %s was restarted: initial UID %s, current UID %s", pod.Name, initialUID, pod.UID)
						}
					}
					return nil
				}),
			},

			// Note: We don't call CheckKeystoreReloadConverged here because calling the reload API
			// would trigger a reload as a side effect, potentially masking operator bugs.
			// The CheckESKeystoreEntries above already verifies the keystore file has the correct entries.

			// Cleanup
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSettings)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			},
		}).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}

// TestReloadableKeystoreDisabled tests that the reloadable keystore feature can be disabled
// via annotation, falling back to the init container approach.
func TestReloadableKeystoreDisabled(t *testing.T) {
	// Skip if version is below 9.3.0
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	if stackVersion.LT(keystorejob.MinVersion) {
		t.Skipf("Skipping reloadable keystore test: version %s is below minimum %s", stackVersion, keystorejob.MinVersion)
	}

	k := test.NewK8sClientOrFatal()

	const securePasswordSettingKey = "xpack.notification.email.account.foo.smtp.secure_password"

	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disabled-reloadable-keystore-secrets",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			securePasswordSettingKey: []byte("test_password"),
		},
	}

	// Set up a cluster with the reloadable keystore feature disabled
	b := elasticsearch.NewBuilder("test-ks-disabled").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettings.Name).
		WithAnnotation(esv1.DisableReloadableKeystoreAnnotation, "true")

	test.StepList{}.
		WithStep(test.Step{
			Name: "Create secure settings secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSettings)
			}),
		}).
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithSteps(test.StepList{
			// Verify secure settings are in the keystore (using traditional init container)
			elasticsearch.CheckESKeystoreEntries(k, b, []string{
				securePasswordSettingKey,
			}),

			// Verify keystore secret does NOT exist (since we're using init container approach)
			test.Step{
				Name: "Verify keystore secret does NOT exist (using init container approach)",
				Test: test.Eventually(func() error {
					var keystoreSecret corev1.Secret
					secretName := esv1.KeystoreSecretName(b.Elasticsearch.Name)
					nsn := types.NamespacedName{Namespace: b.Elasticsearch.Namespace, Name: secretName}
					err := k.Client.Get(context.Background(), nsn, &keystoreSecret)
					if apierrors.IsNotFound(err) {
						return nil // Expected: secret should not exist
					}
					if err != nil {
						return err
					}
					return fmt.Errorf("keystore secret %s should not exist when feature is disabled", secretName)
				}),
			},

			// Cleanup
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSettings)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			},
		}).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}
