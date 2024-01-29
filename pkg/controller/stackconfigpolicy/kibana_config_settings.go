// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	commonlabels "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	KibanaConfigKey = "kibana.json"
)

func newKibanaConfigSecret(policy policyv1alpha1.StackConfigPolicy, kibana kibanav1.Kibana) (corev1.Secret, error) {
	kibanaConfigHash := getKibanaConfigHash(policy.Spec.Kibana.Config)
	configDataJSONBytes := []byte("")
	var err error
	if policy.Spec.Kibana.Config != nil {
		if configDataJSONBytes, err = policy.Spec.Kibana.Config.MarshalJSON(); err != nil {
			return corev1.Secret{}, err
		}
	}
	kibanaConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kibana.Namespace,
			Name:      GetPolicyConfigSecretName(kibana.Name),
			Labels: kblabel.NewLabels(types.NamespacedName{
				Name:      kibana.Name,
				Namespace: kibana.Namespace,
			}),
			Annotations: map[string]string{
				commonannotation.KibanaConfigHashAnnotation: kibanaConfigHash,
			},
		},
		Data: map[string][]byte{
			KibanaConfigKey: configDataJSONBytes,
		},
	}

	// Set policy as the soft owner
	filesettings.SetSoftOwner(&kibanaConfigSecret, policy)

	// Add label to delete secret on deletion of the stack config policy
	kibanaConfigSecret.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

	// Add SecureSettings as annotation
	if err = setKibanaSecureSettings(&kibanaConfigSecret, policy); err != nil {
		return kibanaConfigSecret, err
	}

	return kibanaConfigSecret, nil
}

func getKibanaConfigHash(kibanaConfig *commonv1.Config) string {
	if kibanaConfig != nil {
		return hash.HashObject(kibanaConfig)
	}
	return ""
}

func GetPolicyConfigSecretName(kibanaName string) string {
	return kibanaName + "-kb-policy-config"
}

func kibanaConfigApplied(c k8s.Client, policy policyv1alpha1.StackConfigPolicy, kb kibanav1.Kibana) (bool, error) {
	existingKibanaPods, err := k8s.PodsMatchingLabels(c, kb.Namespace, map[string]string{"kibana.k8s.elastic.co/name": kb.Name})
	if err != nil || len(existingKibanaPods) == 0 {
		return false, err
	}

	kibanaConfigHash := getKibanaConfigHash(policy.Spec.Kibana.Config)
	for _, kbPod := range existingKibanaPods {
		if kbPod.Annotations[commonannotation.KibanaConfigHashAnnotation] != kibanaConfigHash {
			return false, nil
		}
	}

	return true, nil
}

func canBeOwned(ctx context.Context, c k8s.Client, policy policyv1alpha1.StackConfigPolicy, kb kibanav1.Kibana) (reconciler.SoftOwnerRef, bool, error) {
	// Check if the secret already exists
	var kibanaConfigSecret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{
		Name:      GetPolicyConfigSecretName(kb.Name),
		Namespace: kb.Namespace,
	}, &kibanaConfigSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return reconciler.SoftOwnerRef{}, false, err
	}

	if apierrors.IsNotFound(err) {
		// Secret does not exist, return true
		return reconciler.SoftOwnerRef{}, true, nil
	}

	currentOwner, referenced := reconciler.SoftOwnerRefFromLabels(kibanaConfigSecret.Labels)
	// either there is no soft owner
	if !referenced {
		return currentOwner, true, nil
	}
	// or the owner is already the given policy
	canBeOwned := currentOwner.Kind == policyv1alpha1.Kind && currentOwner.Namespace == policy.Namespace && currentOwner.Name == policy.Name
	return currentOwner, canBeOwned, nil
}

// setKibanaSecureSettings stores the SecureSettings Secret sources referenced in the given StackConfigPolicy for Kibana in the annotation of the Kibana config Secret.
func setKibanaSecureSettings(settingsSecret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) error {
	if len(policy.Spec.Kibana.SecureSettings) == 0 {
		return nil
	}

	var secretSources []commonv1.NamespacedSecretSource //nolint:prealloc
	// SecureSettings field under Kibana in the StackConfigPolicy
	for _, src := range policy.Spec.Kibana.SecureSettings {
		secretSources = append(secretSources, commonv1.NamespacedSecretSource{Namespace: policy.GetNamespace(), SecretName: src.SecretName, Entries: src.Entries})
	}

	bytes, err := json.Marshal(secretSources)
	if err != nil {
		return err
	}
	settingsSecret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName] = string(bytes)
	return nil
}
