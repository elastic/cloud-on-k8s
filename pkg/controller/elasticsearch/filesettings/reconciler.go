// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
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

// ReconcileEmptyFileSettingsSecret reconciles an empty File settings Secret for the given Elasticsearch only when there is no Secret.
// Used by the Elasticsearch controller.
func ReconcileEmptyFileSettingsSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	createOnly bool,
) error {
	var currentSecret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)}, &currentSecret)
	// do nothing when Secret already exists and create only
	if err == nil && createOnly {
		return nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Pass the current secret so that stateless cluster_secrets are preserved when
	// SCP-managed fields are cleared.
	var currentSecretPtr *corev1.Secret
	if err == nil {
		currentSecretPtr = &currentSecret
	}

	meta := metadata.Propagate(&es, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&es))})
	expectedSecret, _, err := NewSettingsSecretWithVersion(ctx, k8s.ExtractNamespacedName(&es), es.IsStateless(), currentSecretPtr, nil, nil, meta)
	if err != nil {
		return err
	}

	return reconcileSecret(ctx, c, expectedSecret, &es, fileSettingsManagedAnnotations)
}

// ReconcileFileSettingsSecret reconciles the file settings Secret for the given owner.
func ReconcileFileSettingsSecret(ctx context.Context, c k8s.Client, expected corev1.Secret, owner client.Object) error {
	return reconcileSecret(ctx, c, expected, owner, fileSettingsManagedAnnotations)
}

// ReconcileESConfigSecret reconciles the Elasticsearch config Secret for the given owner.
func ReconcileESConfigSecret(ctx context.Context, c k8s.Client, expected corev1.Secret, owner client.Object) error {
	return reconcileSecret(ctx, c, expected, owner, esConfigManagedAnnotations)
}

// ReconcileKibanaConfigSecret reconciles the Kibana config Secret for the given owner.
func ReconcileKibanaConfigSecret(ctx context.Context, c k8s.Client, expected corev1.Secret, owner client.Object) error {
	return reconcileSecret(ctx, c, expected, owner, kibanaConfigManagedAnnotations)
}

// reconcileSecret reconciles the given Secret.
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
	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// Secrets must be updated in the following cases:
			// * the expected labels/annotations are not a subset of the existing labels/annotations,
			// * the managed labels/annotations have been removed/changed,
			// * the data itself has changed.
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
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			// remove managed labels if they are no longer defined
			for _, label := range managedLabels {
				if _, ok := expected.Labels[label]; !ok {
					delete(reconciled.Labels, label)
				}
			}
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			// remove managed annotations if they are no longer defined
			for _, annotation := range managedAnnotations {
				if _, ok := expected.Annotations[annotation]; !ok {
					delete(reconciled.Annotations, annotation)
				}
			}
			reconciled.Data = expected.Data
		},
	})
}
