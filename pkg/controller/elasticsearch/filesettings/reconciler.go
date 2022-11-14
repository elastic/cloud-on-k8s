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

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
	expected, err := NewSettingsSecret(nil, k8s.ExtractNamespacedName(&es), nil)
	if err != nil {
		return err
	}

	return ReconcileSecret(ctx, c, expected.Secret, es)
}

// ReconcileSecret reconciles the given file settings Secret for the given Elasticsearch.
// reconciler.ReconcileSecret is not used because its usage is for custom-provided Secret where we want to preserve
// existing annotations. Here we need to be able to reset annotations to reset a file settings Secret.
func ReconcileSecret(
	ctx context.Context,
	c k8s.Client,
	expected corev1.Secret,
	es esv1.Elasticsearch,
) error {
	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
}
