// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pdb

import (
	"reflect"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Reconcile ensures that a PodDisruptionBudget exists for this cluster according to the spec.
// If the spec has disabled the default PDB, it will ensure it does not exist.
func Reconcile(k8sClient k8s.Client, scheme *runtime.Scheme, es v1alpha1.Elasticsearch) error {
	expected, err := expectedPDB(k8sClient, es)
	if err != nil {
		return err
	}
	if expected == nil {
		return deleteDefaultPDB(k8sClient, es)
	}

	var reconciled v1beta1.PodDisruptionBudget
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     k8sClient,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   expected,
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

// deleteDefaultPDB deletes the default pdb if it exists.
func deleteDefaultPDB(k8sClient k8s.Client, es v1alpha1.Elasticsearch) error {
	// we do this by getting first because that is a local cache read,
	// versus a Delete call, which would hit the API.
	pdb := v1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      name.DefaultPodDisruptionBudget(es.Name),
		},
	}
	if err := k8sClient.Get(k8s.ExtractNamespacedName(&pdb), &pdb); err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		// already deleted, which is fine
		return nil
	}
	if err := k8sClient.Delete(&pdb); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

// expectedPDB returns a PDB according to the given ES spec.
// It may return nil if the PDB has been explicitly disabled in the ES spec.
func expectedPDB(k8sClient k8s.Client, es v1alpha1.Elasticsearch) (*v1beta1.PodDisruptionBudget, error) {
	template := es.Spec.PodDisruptionBudget.DeepCopy()
	if template.IsDisabled() {
		return nil, nil
	}
	if template == nil {
		template = &commonv1alpha1.PodDisruptionBudgetTemplate{}
	}

	expected := v1beta1.PodDisruptionBudget{
		ObjectMeta: template.ObjectMeta,
	}

	// inherit user-provided ObjectMeta, but set our own name & namespace
	expected.Name = name.DefaultPodDisruptionBudget(es.Name)
	expected.Namespace = es.Namespace
	// and append our labels
	expected.Labels = defaults.SetDefaultLabels(expected.Labels, label.NewLabels(k8s.ExtractNamespacedName(&es)))

	if template.Spec.Selector != nil || template.Spec.MaxUnavailable != nil || template.Spec.MinAvailable != nil {
		// use the user-defined spec
		expected.Spec = template.Spec
		return &expected, nil
	}

	// set our default spec
	var err error
	if expected.Spec, err = buildPDBSpec(k8sClient, k8s.ExtractNamespacedName(&es)); err != nil {
		return nil, err
	}

	return &expected, nil
}

// buildPDBSpec returns a PDBSpec computed from the current StatefulSets so that:
// - only one pod is authorized to be disrupted
func buildPDBSpec(k8sClient k8s.Client, es types.NamespacedName) (v1beta1.PodDisruptionBudgetSpec, error) {
	actualSsets, err := sset.RetrieveActualStatefulSets(k8sClient, es)
	if err != nil {
		return v1beta1.PodDisruptionBudgetSpec{}, err
	}
	// the theoretical node count may not match pods currently running (ie. we may miss some), that's fine.
	nodeCount := actualSsets.ExpectedPodCount()
	// authorize only one pod to be disrupted.
	minAvailable := nodeCount - 1
	minAvailableIntStr := intstr.FromInt(int(minAvailable))

	return v1beta1.PodDisruptionBudgetSpec{
		// match all pods for this cluster
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				label.ClusterNameLabelName: es.Name,
			},
		},
		MinAvailable: &minAvailableIntStr,
		// MaxUnavailable can only be used if the selector matches a builtin controller selector
		// (eg. Deployments, StatefulSets, etc.). We cannot use it with our own cluster-name selector.
		MaxUnavailable: nil,
	}, nil
}
