// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package daemonset

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type Params struct {
	PodTemplate corev1.PodTemplateSpec
	Name        string
	Owner       metav1.Object
	Labels      map[string]string
	Selectors   map[string]string
	Strategy    appsv1.DaemonSetUpdateStrategy
}

func New(params Params) appsv1.DaemonSet {
	return appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Owner.GetNamespace(),
			Labels:    params.Labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selectors,
			},
			Template:       params.PodTemplate,
			UpdateStrategy: params.Strategy,
		},
	}
}

// Reconcile creates or updates the given daemon set for the specified owner.
func Reconcile(
	k8sClient k8s.Client,
	expected appsv1.DaemonSet,
	owner client.Object,
) (appsv1.DaemonSet, error) {
	// label the daemon set with a hash of itself
	expected = WithTemplateHash(expected)

	reconciled := &appsv1.DaemonSet{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     k8sClient,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			// compare hash of the DaemonSet at the time it was built
			return hash.GetTemplateHashLabel(reconciled.Labels) != hash.GetTemplateHashLabel(expected.Labels)
		},
		UpdateReconciled: func() {
			expected.DeepCopyInto(reconciled)
		},
	})
	return *reconciled, err
}

// WithTemplateHash returns a new DaemonSet with a hash of its template to ease comparisons.
func WithTemplateHash(d appsv1.DaemonSet) appsv1.DaemonSet {
	dCopy := *d.DeepCopy()
	dCopy.Labels = hash.SetTemplateHashLabel(dCopy.Labels, dCopy)
	return dCopy
}
