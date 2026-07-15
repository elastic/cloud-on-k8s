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
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// GetSecureSettingsSecretSourcesForResources returns validated secure settings sources for the
// given resource. Each returned source is cross-checked against the exact set declared by the
// active StackConfigPolicies that govern this resource.
func GetSecureSettingsSecretSourcesForResources(ctx context.Context, kubeClient k8s.Client, resource metav1.Object, resourceKind string, operatorNamespace string) ([]commonv1.NamespacedSecretSource, error) {
	var sources []commonv1.NamespacedSecretSource
	var err error
	switch resourceKind {
	case esv1.Kind:
		sources, err = filesettings.GetSecureSettingsSecretSources(ctx, kubeClient, resource)
	case kbv1.Kind:
		sources, err = getKibanaSecureSettingsSecretSources(ctx, kubeClient, resource)
	default:
		return []commonv1.NamespacedSecretSource{}, nil
	}
	if err != nil {
		return nil, err
	}
	return filterByAllowedSources(ctx, kubeClient, resource, operatorNamespace, resourceKind, sources)
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

// allowedSourceKeys returns the {namespace, secretName} set declared by policy for the given resource kind.
func allowedSourceKeys(policy *policyv1alpha1.StackConfigPolicy, resourceKind string) map[types.NamespacedName]struct{} {
	var sources []commonv1.NamespacedSecretSource
	switch resourceKind {
	case kbv1.Kind:
		sources = policy.GetKibanaNamespacedSecureSettings()
	case esv1.Kind:
		sources = policy.GetElasticsearchNamespacedSecureSettings()
		// The deprecated top-level Spec.SecureSettings also applies to Elasticsearch and
		// is still written to the annotation by the SCP controller. Include it so that
		// sources from this field are not falsely rejected while the field is still supported.
		for _, s := range policy.Spec.SecureSettings { //nolint:staticcheck
			sources = append(sources, commonv1.NamespacedSecretSource{Namespace: policy.Namespace, SecretName: s.SecretName})
		}
	}
	keys := make(map[types.NamespacedName]struct{}, len(sources))
	for _, s := range sources {
		keys[types.NamespacedName{Namespace: s.Namespace, Name: s.SecretName}] = struct{}{}
	}
	return keys
}

// filterByAllowedSources retains only those sources whose {namespace, secretName} pair is
// declared by an active StackConfigPolicy governing this resource. Sources not present in
// any governing SCP are dropped.
func filterByAllowedSources(ctx context.Context, kubeClient k8s.Client, resource metav1.Object, operatorNamespace string, resourceKind string, sources []commonv1.NamespacedSecretSource) ([]commonv1.NamespacedSecretSource, error) {
	if len(sources) == 0 {
		return sources, nil
	}

	// Only SCPs in the resource's own namespace or the operator namespace can ever
	// govern this resource (DoesPolicyMatchObject rejects all others), so restrict
	// the list to those two namespaces instead of fetching cluster-wide.
	namespacesToCheck := []string{resource.GetNamespace()}
	if operatorNamespace != "" && operatorNamespace != resource.GetNamespace() {
		namespacesToCheck = append(namespacesToCheck, operatorNamespace)
	}

	// Build the allowed set while listing — no intermediate slice needed.
	allowed := map[types.NamespacedName]struct{}{}
	for _, ns := range namespacesToCheck {
		var policyList policyv1alpha1.StackConfigPolicyList
		if err := kubeClient.List(ctx, &policyList, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		for i := range policyList.Items {
			policy := &policyList.Items[i]
			matches, err := DoesPolicyMatchObject(policy, resource, operatorNamespace)
			if err != nil {
				return nil, err
			}
			if !matches {
				continue
			}
			for k := range allowedSourceKeys(policy, resourceKind) {
				allowed[k] = struct{}{}
			}
		}
	}

	log := ulog.FromContext(ctx)
	var validated []commonv1.NamespacedSecretSource
	for _, src := range sources {
		if _, ok := allowed[types.NamespacedName{Namespace: src.Namespace, Name: src.SecretName}]; ok {
			validated = append(validated, src)
		} else {
			log.Info("Ignoring secure settings source: not declared by any active StackConfigPolicy",
				"rejected_namespace", src.Namespace,
				"rejected_name", src.SecretName,
			)
		}
	}
	if validated == nil {
		return []commonv1.NamespacedSecretSource{}, nil
	}
	return validated, nil
}
