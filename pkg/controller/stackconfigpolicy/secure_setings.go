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
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func GetSecureSettingsSecretSourcesForResources(ctx context.Context, kubeClient k8s.Client, resource metav1.Object, resourceKind string) ([]commonv1.NamespacedSecretSource, error) {
	switch resourceKind {
	case "Elasticsearch":
		return filesettings.GetSecureSettingsSecretSources(ctx, kubeClient, resource)
	case "Kibana":
		return getKibanaSecureSettingsSecretSources(ctx, kubeClient, resource)
	default:
		// Just return empty since there are no other resource type monitored by the stack config policy
		return []commonv1.NamespacedSecretSource{}, nil
	}
}

func getKibanaSecureSettingsSecretSources(ctx context.Context, kubeClient k8s.Client, resource metav1.Object) ([]commonv1.NamespacedSecretSource, error) {
	var secret corev1.Secret
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: resource.GetNamespace(), Name: GetPolicyConfigSecretName(resource.GetName())}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return []commonv1.NamespacedSecretSource{}, nil
		}
		return nil, err
	}

	rawString, ok := secret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName]
	if !ok {
		return []commonv1.NamespacedSecretSource{}, nil
	}
	var secretSources []commonv1.NamespacedSecretSource
	if err := json.Unmarshal([]byte(rawString), &secretSources); err != nil {
		return nil, err
	}

	return secretSources, nil
}
