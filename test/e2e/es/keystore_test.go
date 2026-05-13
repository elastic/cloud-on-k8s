// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonkeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

func TestUpdateESSecureSettings(t *testing.T) {
	k := test.NewK8sClientOrFatal()

	// user-provided secure settings secret
	const securePasswordSettingKey = "xpack.notification.email.account.foo.smtp.secure_password"
	const secureBarUserSettingKey = "xpack.notification.jira.account.bar.secure_user"
	const secureBazUserSettingKey = "xpack.notification.jira.account.baz.secure_user"
	secureSettings1 := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-secrets-1",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise ES refuses to start
			securePasswordSettingKey: []byte("foo_pw"),
		},
	}
	secureSettings2 := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-secrets-2",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise ES refuses to start
			secureBarUserSettingKey: []byte("bar_user"),
		},
	}

	secureSettings := []corev1.Secret{secureSettings1, secureSettings2}

	// set up a 3-nodes cluster with secure settings
	b := elasticsearch.NewBuilder("test-es-keystore").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettings1.Name, secureSettings2.Name)

	test.StepList{}.
		// create secure settings secret
		WithStep(test.Step{
			Name: "Create secure settings secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSettings...)
			}),
		}).

		// create the cluster
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithSteps(test.StepList{
			// initial secure settings should be there in all nodes keystore
			elasticsearch.CheckESKeystoreEntries(k, b, []string{
				securePasswordSettingKey,
				secureBarUserSettingKey}),

			// modify the secure settings secret
			test.Step{
				Name: "Modify secure settings secret",
				Test: test.Eventually(func() error {
					// remove some keys, add new ones
					secureSettings2.Data = map[string][]byte{
						secureBazUserSettingKey: []byte("baz"), // the actual value update cannot be checked :(
					}
					return k.Client.Update(context.Background(), &secureSettings2)
				}),
			},
			// keystore should be updated accordingly
			elasticsearch.CheckESKeystoreEntries(k, b, []string{
				securePasswordSettingKey,
				secureBazUserSettingKey,
			}),
			// remove one secret
			test.Step{
				Name: "Remove one of the source secrets",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSettings2)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					return nil
				}),
			},
			// keystore should be updated accordingly
			elasticsearch.CheckESKeystoreEntries(k, b, []string{
				securePasswordSettingKey,
			}),
			// remove the secure settings reference
			test.Step{
				Name: "Remove secure settings from the spec",
				Test: test.Eventually(func() error {
					// retrieve current Elasticsearch resource
					var currentEs esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Elasticsearch), &currentEs); err != nil {
						return err
					}
					// set its secure settings to nil
					currentEs.Spec.SecureSettings = nil
					return k.Client.Update(context.Background(), &currentEs)
				}),
			},

			// keystore should be updated accordingly
			elasticsearch.CheckESKeystoreEntries(k, b, nil),

			// cleanup extra resources
			test.Step{
				Name: "Delete secure settings secret",
				Test: test.Eventually(func() error {
					err := k.Client.Delete(context.Background(), &secureSettings1) // we deleted the other one above already
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

// TestESKeystoreBatchAddAtScale exercises the keystore init container with a
// non-trivial number of secure settings to validate that ECK adds them to the
// Elasticsearch keystore in a single batched `elasticsearch-keystore add-file`
// invocation rather than once per key. See https://github.com/elastic/cloud-on-k8s/issues/9439.
//
// The test:
//   - creates a single secure-settings Secret with N=25 valid xpack.notification
//     entries (mirroring how StackConfigPolicy aggregates many secrets into one),
//   - starts a single-node Elasticsearch cluster referencing that Secret,
//   - asserts that all N keys are present in the keystore on every Pod, and
//   - asserts that the rendered init container script uses the batched form
//     (a regression guard against accidentally reverting to the per-file loop,
//     which incurs a JVM startup per key and dominates pod startup at scale).
func TestESKeystoreBatchAddAtScale(t *testing.T) {
	k := test.NewK8sClientOrFatal()

	const n = 25
	keys, secretData := generateTestSecureSettings(n)

	secureSettings := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("es-keystore-batch-secrets-%s", rand.String(4)),
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: secretData,
	}

	b := elasticsearch.NewBuilder("test-es-keystore-batch").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithESSecureSettings(secureSettings.Name)

	test.StepList{}.
		WithStep(test.Step{
			Name: fmt.Sprintf("Create secure settings secret with %d entries", n),
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSettings)
			}),
		}).
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithStep(elasticsearch.CheckESKeystoreEntries(k, b, keys)).
		WithStep(checkKeystoreInitScriptIsBatched(k, b.Elasticsearch)).
		WithSteps(b.DeletionTestSteps(k)).
		WithStep(test.Step{
			Name: "Delete secure settings secret",
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), &secureSettings)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		}).
		RunSequential(t)
}

// generateTestSecureSettings returns n valid xpack.notification secure setting
// names and a corresponding Secret data map populated with inert test fixtures
// (not real credentials). Names are returned sorted to match the order
// Elasticsearch reports them via `elasticsearch-keystore list`.
func generateTestSecureSettings(n int) ([]string, map[string][]byte) {
	keys := make([]string, 0, n)
	data := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		// xpack.notification.email accepts arbitrary user-defined account
		// names, so we can mint as many valid secure settings as we want.
		key := fmt.Sprintf("xpack.notification.email.account.acct%02d.smtp.secure_password", i)
		keys = append(keys, key)
		data[key] = []byte(fmt.Sprintf("test-fixture-%02d", i))
	}
	sort.Strings(keys)
	return keys, data
}

// checkKeystoreInitScriptIsBatched asserts that the keystore init container's
// rendered script issues a single `elasticsearch-keystore add-file` invocation
// over a bash array of (setting, path) pairs, rather than one invocation per
// secret. This guards against accidentally reverting to the legacy per-file
// loop introduced before https://github.com/elastic/cloud-on-k8s/issues/9439.
func checkKeystoreInitScriptIsBatched(k *test.K8sClient, es esv1.Elasticsearch) test.Step {
	return test.Step{
		Name: "Keystore init container script should batch add-file invocations",
		Test: test.Eventually(func() error {
			if len(es.Spec.NodeSets) == 0 {
				return fmt.Errorf("expected at least one nodeset")
			}
			var sset appsv1.StatefulSet
			if err := k.Client.Get(
				context.Background(),
				types.NamespacedName{Namespace: es.Namespace, Name: esv1.StatefulSet(es.Name, es.Spec.NodeSets[0].Name)},
				&sset,
			); err != nil {
				return err
			}
			init := pod.InitContainerByName(sset.Spec.Template.Spec, commonkeystore.InitContainerName)
			if init == nil {
				return fmt.Errorf("init container %q not found", commonkeystore.InitContainerName)
			}
			script := strings.Join(init.Command, " ")
			if !strings.Contains(script, `add-file "${add_args[@]}"`) {
				return fmt.Errorf("expected batched `add-file \"${add_args[@]}\"` form in keystore init script, got: %s", script)
			}
			if strings.Contains(script, `add-file "$key" "$filename"`) {
				return fmt.Errorf("legacy per-file `add-file \"$key\" \"$filename\"` form unexpectedly present in keystore init script: %s", script)
			}
			return nil
		}),
	}
}
