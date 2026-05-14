// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"sort"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
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
		// secureSettings1 is populated below with N>1 entries to also exercise
		// the batched `elasticsearch-keystore add-file` path (#9439). The keys
		// generated here are merged with the well-known securePasswordSettingKey
		// so the existing assertion that exact-checks for that key still holds.
		Data: map[string][]byte{
			// this needs to be a valid configuration item, otherwise ES refuses to start
			securePasswordSettingKey: []byte("foo_pw"),
		},
	}
	// Add additional valid xpack.notification.email entries so the keystore is
	// initialized with a non-trivial number of secure settings on each Pod.
	// This exercises the batched add-file invocation introduced in #9440 in
	// the same end-to-end run, without the cost of a dedicated test cluster.
	const extraEntries = 24 // 24 + securePasswordSettingKey == 25 total in secureSettings1
	extraKeys, extraData := generateExtraSecureSettings(extraEntries)
	for k, v := range extraData {
		secureSettings1.Data[k] = v
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

	// initialKeys is the full set of keys we expect to find in the keystore
	// once both secrets are referenced (sorted, since the e2e helper compares
	// against `elasticsearch-keystore list`'s sorted output).
	initialKeys := append([]string{securePasswordSettingKey, secureBarUserSettingKey}, extraKeys...)
	sort.Strings(initialKeys)

	// after secureSettings2 is updated to drop bar and add baz
	updatedKeys := append([]string{securePasswordSettingKey, secureBazUserSettingKey}, extraKeys...)
	sort.Strings(updatedKeys)

	// after secureSettings2 is deleted
	postDeleteKeys := append([]string{securePasswordSettingKey}, extraKeys...)
	sort.Strings(postDeleteKeys)

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
			elasticsearch.CheckESKeystoreEntries(k, b, initialKeys),

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
			elasticsearch.CheckESKeystoreEntries(k, b, updatedKeys),
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
			elasticsearch.CheckESKeystoreEntries(k, b, postDeleteKeys),
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

// generateExtraSecureSettings returns n valid xpack.notification secure
// setting names plus an inert test-fixture value for each one. Used to bulk
// up TestUpdateESSecureSettings with enough entries to exercise the batched
// `elasticsearch-keystore add-file` path on a real cluster (#9439). The
// values are not credentials; xpack.notification.email accepts arbitrary
// account names so we can mint as many valid entries as we want.
func generateExtraSecureSettings(n int) ([]string, map[string][]byte) {
	keys := make([]string, 0, n)
	data := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("xpack.notification.email.account.acct%02d.smtp.secure_password", i)
		keys = append(keys, key)
		data[key] = []byte(fmt.Sprintf("test-fixture-%02d", i))
	}
	return keys, data
}
