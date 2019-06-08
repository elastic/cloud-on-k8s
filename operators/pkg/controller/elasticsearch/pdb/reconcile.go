// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pdb

import (
	"reflect"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Reconcile ensures that a PodDisruptionBudget exists for this cluster according to the spec.
//
// If the spec has disabled the default PDB, it will ensure it does not exist
func Reconcile(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
) error {
	disabled := false

	template := es.Spec.PodDisruptionBudget
	if template != nil {
		// clone to avoid accidentally overwriting template fields
		template = template.DeepCopy()

		emptyTemplate := commonv1alpha1.PodDisruptionBudgetTemplate{}
		if reflect.DeepEqual(&emptyTemplate, template) {
			disabled = true
		}
	} else {
		template = &commonv1alpha1.PodDisruptionBudgetTemplate{}
	}

	var objectMeta v1.ObjectMeta
	if template != nil {
		objectMeta = *template.ObjectMeta.DeepCopy()
	}
	objectMeta.Name = name.DefaultPodDisruptionBudget(es.Name)
	objectMeta.Namespace = es.Namespace
	objectMeta.Labels = defaults.SetDefaultLabels(objectMeta.Labels, label.NewLabels(k8s.ExtractNamespacedName(&es)))

	expected := v1beta1.PodDisruptionBudget{
		ObjectMeta: objectMeta,
		Spec:       template.Spec,
	}

	if disabled {
		// delete the default budget if it exists.
		//
		// we do this by getting first because that is a local cache read,
		// versus a Delete call, which would hit the API.

		if err := c.Get(k8s.ExtractNamespacedName(&expected), &expected); err != nil && !errors.IsNotFound(err) {
			return err
		} else if errors.IsNotFound(err) {
			// already deleted, which is fine
			return nil
		}

		if err := c.Delete(&expected); err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	// set our defaults
	if expected.Spec.MaxUnavailable == nil {
		expected.Spec.MaxUnavailable = &commonv1alpha1.DefaultPodDisruptionBudgetMaxUnavailable
	}
	if expected.Spec.Selector == nil {
		expected.Spec.Selector = &v1.LabelSelector{
			MatchLabels: map[string]string{
				label.ClusterNameLabelName: es.Name,
			},
		}
	}

	var reconciled v1beta1.PodDisruptionBudget
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			for k, v := range expected.Labels {
				if rv, ok := reconciled.Labels[k]; !ok || rv != v {
					return true
				}
			}

			return !reflect.DeepEqual(expected.Spec, reconciled.Spec)
		},
		UpdateReconciled: func() {
			for k, v := range expected.Labels {
				reconciled.Labels[k] = v
			}
			reconciled.Spec = expected.Spec
		},
	})
}
