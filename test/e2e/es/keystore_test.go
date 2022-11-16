// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
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
