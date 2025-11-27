// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	commonlabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	KibanaConfigKey = "kibana.json"
)

func newKibanaConfigSecret(
	kbConfigPolicy policyv1alpha1.KibanaConfigPolicySpec,
	namespacedSecretSources []commonv1.NamespacedSecretSource,
	kibana kibanav1.Kibana,
	policyRefs []policyv1alpha1.StackConfigPolicy,
) (corev1.Secret, error) {
	kibanaConfigHash := getKibanaConfigHash(kbConfigPolicy.Config)
	configDataJSONBytes := []byte("")
	var err error
	if kbConfigPolicy.Config != nil {
		if configDataJSONBytes, err = kbConfigPolicy.Config.MarshalJSON(); err != nil {
			return corev1.Secret{}, err
		}
	}
	meta := metadata.Propagate(&kibana, metadata.Metadata{
		Labels: kblabel.NewLabels(k8s.ExtractNamespacedName(&kibana)),
		Annotations: map[string]string{
			commonannotation.KibanaConfigHashAnnotation: kibanaConfigHash,
		},
	})
	kibanaConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   kibana.Namespace,
			Name:        GetPolicyConfigSecretName(kibana.Name),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			KibanaConfigKey: configDataJSONBytes,
		},
	}

	// Add label to delete secret on deletion of the stack config policy
	kibanaConfigSecret.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

	// Add SecureSettings as annotation
	if err = setKibanaSecureSettings(&kibanaConfigSecret, namespacedSecretSources); err != nil {
		return kibanaConfigSecret, err
	}

	if err = setMultipleSoftOwners(&kibanaConfigSecret, policyRefs); err != nil {
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

func kibanaConfigApplied(c k8s.Client, kbConfigPolicy policyv1alpha1.KibanaConfigPolicySpec, kb kibanav1.Kibana) (bool, error) {
	existingKibanaPods, err := k8s.PodsMatchingLabels(c, kb.Namespace, map[string]string{"kibana.k8s.elastic.co/name": kb.Name})
	if err != nil || len(existingKibanaPods) == 0 {
		return false, err
	}

	kibanaConfigHash := getKibanaConfigHash(kbConfigPolicy.Config)
	for _, kbPod := range existingKibanaPods {
		if kbPod.Annotations[commonannotation.KibanaConfigHashAnnotation] != kibanaConfigHash {
			return false, nil
		}
	}

	return true, nil
}

// setKibanaSecureSettings stores the SecureSettings Secret sources referenced in the given StackConfigPolicy for Kibana in the annotation of the Kibana config Secret.
func setKibanaSecureSettings(settingsSecret *corev1.Secret, namespacedSecretSources []commonv1.NamespacedSecretSource) error {
	if len(namespacedSecretSources) == 0 {
		return nil
	}

	bytes, err := json.Marshal(namespacedSecretSources)
	if err != nil {
		return err
	}
	settingsSecret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName] = string(bytes)
	return nil
}
