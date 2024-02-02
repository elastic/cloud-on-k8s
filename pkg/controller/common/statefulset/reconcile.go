// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package statefulset

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// Params to specify a StatefulSet specification.
type Params struct {
	Name                 string
	Namespace            string
	ServiceName          string
	Selector             map[string]string
	Labels               map[string]string
	PodTemplateSpec      corev1.PodTemplateSpec
	VolumeClaimTemplates []corev1.PersistentVolumeClaim
	Replicas             int32
	PodManagementPolicy  appsv1.PodManagementPolicyType
	RevisionHistoryLimit *int32
}

// New creates a StatefulSet from the given params.
func New(params Params) appsv1.StatefulSet {
	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &params.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},
			Template:             params.PodTemplateSpec,
			VolumeClaimTemplates: params.VolumeClaimTemplates,
			ServiceName:          params.ServiceName,
			RevisionHistoryLimit: params.RevisionHistoryLimit,
			PodManagementPolicy:  params.PodManagementPolicy,
			// always RollingUpdate strategy as OnDelete would require the user
			// to delete pods and this is not fully with the operator reconcile logic.
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: nil,
			},
			// consider the pod available as soon as it is ready
			MinReadySeconds: 0,
			// agent in statefulSet requires no PVC retention policies
			PersistentVolumeClaimRetentionPolicy: nil,
			// agent in statefulSet requires no ordinals
			Ordinals: nil,
		},
	}
}

// Reconcile creates or updates the given StatefulSet for the specified owner.
func Reconcile(
	ctx context.Context,
	k8sClient k8s.Client,
	expected appsv1.StatefulSet,
	owner client.Object,
) (appsv1.StatefulSet, error) {
	// label the StatefulSet with a hash of itself
	expected = WithTemplateHash(expected)

	reconciled := &appsv1.StatefulSet{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     k8sClient,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// compare hash of the StatefulSet at the time it was built
			return hash.GetTemplateHashLabel(reconciled.Labels) != hash.GetTemplateHashLabel(expected.Labels)
		},
		UpdateReconciled: func() {
			// set expected annotations and labels, but don't remove existing ones
			// that may have been defaulted or set by a user/admin on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			// overwrite the spec but leave the status intact
			reconciled.Spec = expected.Spec
		},
	})
	return *reconciled, err
}

// WithTemplateHash returns a new StatefulSet with a hash of its template to ease comparisons.
func WithTemplateHash(d appsv1.StatefulSet) appsv1.StatefulSet {
	dCopy := *d.DeepCopy()
	dCopy.Labels = hash.SetTemplateHashLabel(dCopy.Labels, dCopy)
	return dCopy
}

// NewPodTemplateValidator returns a function which can be used to validate the PodTemplateSpec in a StatefulSet
func NewPodTemplateValidator(ctx context.Context, c k8s.Client, owner client.Object, expected appsv1.StatefulSet) func() error {
	sset := expected.DeepCopy()
	return func() error {
		return validatePodTemplate(ctx, c, owner, *sset)
	}
}
