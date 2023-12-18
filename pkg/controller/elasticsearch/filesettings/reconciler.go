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

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

var (
	// managedLabels are the labels managed by the operator for the file settings Secret, which means that the operator
	// will always take precedence to update or remove these labels.
	managedLabels = []string{reconciler.SoftOwnerNamespaceLabel, reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerKindLabel}

	// managedAnnotations are the annotations managed by the operator for the stack config policy related secrets, which means that the operator
	// will always take precedence to update or remove these annotations.
	managedAnnotations = []string{commonannotation.SecureSettingsSecretsAnnotationName, commonannotation.SettingsHashAnnotationName, commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation, commonannotation.KibanaConfigHashAnnotation}
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

	// no secret, reconcile a new empty file settings
	expectedSecret, _, err := NewSettingsSecretWithVersion(k8s.ExtractNamespacedName(&es), nil, nil)
	if err != nil {
		return err
	}

	return ReconcileSecret(ctx, c, expectedSecret, &es)
}

// ReconcileSecret reconciles the given Secret.
// This implementation is slightly different from reconciler.ReconcileSecret to allow resetting managed annotations.
func ReconcileSecret(
	ctx context.Context,
	c k8s.Client,
	expected corev1.Secret,
	owner client.Object,
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
			return (!maps.IsSubset(expected.Labels, reconciled.Labels) || !maps.IsEqualSubset(expected.Labels, reconciled.Labels, managedLabels)) ||
				(!maps.IsSubset(expected.Annotations, reconciled.Annotations) || !maps.IsEqualSubset(expected.Annotations, reconciled.Annotations, managedAnnotations)) ||
				!reflect.DeepEqual(expected.Data, reconciled.Data)
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
