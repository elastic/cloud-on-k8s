// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

var (
	// managedLabels are the labels managed by the operator for the file settings Secret, which means that the operator
	// will always take precedence to update or remove these labels.
	managedLabels = []string{reconciler.SoftOwnerNamespaceLabel, reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerKindLabel}

	// fileSettingsManagedAnnotations are the annotations managed by the operator for the file settings Secret.
	fileSettingsManagedAnnotations = []string{commonannotation.SecureSettingsSecretsAnnotationName, commonannotation.SettingsHashAnnotationName, reconciler.SoftOwnerRefsAnnotation}

	// esConfigManagedAnnotations are the annotations managed by the operator for the Elasticsearch config Secret.
	esConfigManagedAnnotations = []string{commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation, reconciler.SoftOwnerRefsAnnotation}

	// kibanaConfigManagedAnnotations are the annotations managed by the operator for the Kibana config Secret.
	kibanaConfigManagedAnnotations = []string{commonannotation.SecureSettingsSecretsAnnotationName, commonannotation.KibanaConfigHashAnnotation, reconciler.SoftOwnerRefsAnnotation}
)

// ReconcileESConfigSecret reconciles the Elasticsearch config Secret for the given owner.
func ReconcileESConfigSecret(ctx context.Context, c k8s.Client, expected corev1.Secret, owner client.Object) error {
	return reconcileSecret(ctx, c, expected, owner, esConfigManagedAnnotations)
}

// ReconcileKibanaConfigSecret reconciles the Kibana config Secret for the given owner.
func ReconcileKibanaConfigSecret(ctx context.Context, c k8s.Client, expected corev1.Secret, owner client.Object) error {
	return reconcileSecret(ctx, c, expected, owner, kibanaConfigManagedAnnotations)
}

// reconcileSecret reconciles the given Secret for Elasticsearch config and Kibana config secrets.
// The file settings Secret is not reconciled here — it is managed by the Secret type in file_settings_secret.go.
// This implementation is slightly different from reconciler.ReconcileSecret to allow resetting managed annotations.
// The managedAnnotations parameter scopes which annotations are actively managed (and removed if absent from expected),
// preventing cross-type annotation drift between different secret types.
func reconcileSecret(
	ctx context.Context,
	c k8s.Client,
	expected corev1.Secret,
	owner client.Object,
	managedAnnotations []string,
) error {
	if expected.Labels == nil {
		expected.Labels = make(map[string]string)
	}
	expected.Labels[commonv1.LabelBasedDiscoveryLabelName] = commonv1.LabelBasedDiscoveryLabelValue

	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// Compare expected against current state (reconciled holds the Get result).
			// Owner refs and managed key cleanup are handled by ReconcileResource separately.
			labelsChanged := !maps.IsSubset(expected.Labels, reconciled.Labels) || !maps.IsEqualSubset(expected.Labels, reconciled.Labels, managedLabels)
			annotationsChanged := !maps.IsSubset(expected.Annotations, reconciled.Annotations) || !maps.IsEqualSubset(expected.Annotations, reconciled.Annotations, managedAnnotations)
			dataChanged := !reflect.DeepEqual(expected.Data, reconciled.Data)
			if labelsChanged || annotationsChanged || dataChanged {
				ulog.FromContext(ctx).V(1).Info("Secret needs update",
					"secret_namespace", expected.Namespace, "secret_name", expected.Name,
					"labels_changed", labelsChanged, "annotations_changed", annotationsChanged, "data_changed", dataChanged,
				)
				return true
			}
			return false
		},
		UpdateReconciled: func() {
			applyExpectedSecret(reconciled, expected, managedAnnotations, false)
		},
	})
}

// applyExpectedSecret applies the expected state onto the reconciled Secret.
// Labels and annotations from expected are merged into reconciled. Data is replaced wholesale.
// When additiveOnly is false (SCP controller), managed labels and annotations that are
// absent from expected are removed from reconciled. When additiveOnly is true
// (ES controller), existing labels and annotations are never removed, only added to.
// Owner references are not handled here — they are set separately via SetControllerReference.
func applyExpectedSecret(reconciled *corev1.Secret, expected corev1.Secret, managedAnnotations []string, additiveOnly bool) {
	reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
	reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
	reconciled.Data = expected.Data

	if additiveOnly {
		return
	}
	// Remove managed labels/annotations that are no longer defined in expected.
	for _, label := range managedLabels {
		if _, ok := expected.Labels[label]; !ok {
			delete(reconciled.Labels, label)
		}
	}
	for _, annotation := range managedAnnotations {
		if _, ok := expected.Annotations[annotation]; !ok {
			delete(reconciled.Annotations, annotation)
		}
	}
}
