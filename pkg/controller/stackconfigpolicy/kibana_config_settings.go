// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	commonlabels "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	KibanaConfigKey            = "kibana.json"
	KibanaConfigHashAnnotation = "policy.k8s.elastic.co/kibana-config-hash"
)

func newKibanaConfigSecret(policy policyv1alpha1.StackConfigPolicy, kibana kibanav1.Kibana) (corev1.Secret, error) {
	kibanaConfigHash := getKibanaConfigHash(policy.Spec.Kibana.Config)
	var configDataJSONBytes []byte
	var err error
	if policy.Spec.Kibana.Config != nil {
		configDataJSONBytes, err = policy.Spec.Elasticsearch.Config.MarshalJSON()
		if err != nil {
			return corev1.Secret{}, err
		}
	}
	kibanaConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kibana.Namespace,
			Name:      GetPolicyConfigSecretName(kibana),
			Labels: kblabel.NewLabels(types.NamespacedName{
				Name:      kibana.Name,
				Namespace: kibana.Namespace,
			}),
			Annotations: map[string]string{
				KibanaConfigHashAnnotation: kibanaConfigHash,
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

	return kibanaConfigSecret, nil
}

func getKibanaConfigHash(kibanaConfig *commonv1.Config) string {
	if kibanaConfig != nil {
		return hash.HashObject(kibanaConfig)
	}
	return ""
}

func GetPolicyConfigSecretName(kibana kibanav1.Kibana) string {
	return kibana.Name + "-kb-policy-config"
}

func kibanaConfigApplied(ctx context.Context, c k8s.Client, policy policyv1alpha1.StackConfigPolicy, kb kibanav1.Kibana) (bool, error) {
	existingKibanaPods, err := k8s.PodsMatchingLabels(c, kb.Namespace, map[string]string{"kibana.k8s.elastic.co/name": kb.Name})
	if err != nil || len(existingKibanaPods) == 0 {
		return false, err
	}

	kibanaConfigHash := getKibanaConfigHash(policy.Spec.Kibana.Config)
	for _, kbPod := range existingKibanaPods {
		if kbPod.Annotations[KibanaConfigHashAnnotation] != kibanaConfigHash {
			return false, nil
		}
	}

	return true, nil
}
