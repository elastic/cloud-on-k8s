// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
)

func AnnotatePodsWithBuilderHash(subj, prev Subject, k *K8sClient) StepList {
	return []Step{
		{
			Name: "Annotate Pods with a hash of their Builder spec",
			Test: Eventually(func() error {
				var pods corev1.PodList
				if err := k.Client.List(context.Background(), &pods, subj.ListOptions()...); err != nil {
					return err
				}

				expectedHash := hash.HashObject(prev.Spec())
				for _, pod := range pods.Items {
					if err := AnnotatePodWithBuilderHash(k, pod, expectedHash); err != nil {
						return err
					}
				}
				return nil
			}),
		},
	}
}

func CreateEnterpriseLicenseSecret(t *testing.T, k *K8sClient, secretName string, licenseBytes []byte) {
	t.Helper()
	Eventually(func() error {
		sec := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: Ctx().ManagedNamespace(0),
				Name:      secretName,
				Labels: map[string]string{
					commonv1.TypeLabelName:    license.Type,
					license.LicenseLabelScope: string(license.LicenseScopeOperator),
				},
			},
			Data: map[string][]byte{
				license.FileName: licenseBytes,
			},
		}
		return k.CreateOrUpdate(&sec)
	})(t)
}

func DeleteAllEnterpriseLicenseSecrets(t *testing.T, k *K8sClient) {
	t.Helper()
	Eventually(func() error {
		// Delete operator license secret
		var licenseSecrets corev1.SecretList
		err := k.Client.List(context.Background(), &licenseSecrets, k8sclient.MatchingLabels(map[string]string{commonv1.TypeLabelName: license.Type}))
		if err != nil {
			return err
		}
		for i := range licenseSecrets.Items {
			err = k.Client.Delete(context.Background(), &licenseSecrets.Items[i])
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	})(t)
}

// LicenseTestBuilder is a wrapped builder for tests that require a valid Enterprise license to be installed in the operator.
// It creates an Enterprise license secret before the test and deletes it again after the test. Callers are responsible for
// making sure that Ctx().TestLicense contains a valid test license.
func LicenseTestBuilder(b Builder) WrappedBuilder {
	return WrappedBuilder{
		BuildingThis: b,
		PreInitSteps: func(k *K8sClient) StepList {
			//nolint:thelper
			return StepList{
				Step{
					Name: "Create an Enterprise license secret",
					Test: func(t *testing.T) {
						licenseBytes, err := os.ReadFile(Ctx().TestLicense)
						require.NoError(t, err)
						DeleteAllEnterpriseLicenseSecrets(t, k)
						CreateEnterpriseLicenseSecret(t, k, "eck-license", licenseBytes)
					},
				},
			}
		},
		PreDeletionSteps: func(k *K8sClient) StepList {
			//nolint:thelper
			return StepList{
				Step{
					Name: "Removing any test enterprise license secrets",
					Test: func(t *testing.T) {
						DeleteAllEnterpriseLicenseSecrets(t, k)
					},
				},
			}
		},
	}
}
