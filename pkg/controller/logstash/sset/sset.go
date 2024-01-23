// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

type Params struct {
	Name                 string
	Namespace            string
	ServiceName          string
	Selector             map[string]string
	Labels               map[string]string
	PodTemplateSpec      corev1.PodTemplateSpec
	VolumeClaimTemplates []corev1.PersistentVolumeClaim
	Replicas             int32
	RevisionHistoryLimit *int32
}

func New(params Params) appsv1.StatefulSet {
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			// we don't care much about pods creation ordering, and manage deletion ordering ourselves,
			// so we're fine with the StatefulSet controller spawning all pods in parallel
			PodManagementPolicy:  appsv1.ParallelPodManagement,
			RevisionHistoryLimit: params.RevisionHistoryLimit,
			// build a headless service per StatefulSet, matching the StatefulSet labels
			ServiceName: params.ServiceName,
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Selector,
			},

			Replicas:             &params.Replicas,
			Template:             params.PodTemplateSpec,
			VolumeClaimTemplates: params.VolumeClaimTemplates,
		},
	}

	// store a hash of the sset resource in its labels for comparison purposes
	sset.Labels = hash.SetTemplateHashLabel(sset.Labels, sset.Spec)

	return sset
}

// Reconcile creates or updates the expected StatefulSet.
func Reconcile(ctx context.Context, c k8s.Client, expected appsv1.StatefulSet, owner client.Object, expectations *expectations.Expectations) (appsv1.StatefulSet, error) {
	var reconciled appsv1.StatefulSet

	err := reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// expected labels or annotations not there
			return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
				!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
				// different spec
				!EqualTemplateHashLabels(expected, reconciled)
		},
		UpdateReconciled: func() {
			// override annotations and labels with expected ones
			// don't remove additional values in reconciled that may have been defaulted or
			// manually set by the user on the existing resource
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
			reconciled.Spec = expected.Spec
		},
		PostUpdate: func() {
			if expectations != nil {
				// expect the reconciled StatefulSet to be there in the cache for next reconciliations,
				// to prevent assumptions based on the wrong replica count
				expectations.ExpectGeneration(reconciled)
			}
		},
	})
	return reconciled, err
}

// EqualTemplateHashLabels reports whether actual and expected StatefulSets have the same template hash label value.
func EqualTemplateHashLabels(expected, actual appsv1.StatefulSet) bool {
	return expected.Labels[hash.TemplateHashLabelName] == actual.Labels[hash.TemplateHashLabelName]
}


// GetReplicas returns the replicas configured for this StatefulSet, or 0 if nil.
func GetReplicas(statefulSet appsv1.StatefulSet) int32 {
	if statefulSet.Spec.Replicas != nil {
		return *statefulSet.Spec.Replicas
	}
	return 0
}

// GetClaim returns a pointer to the claim with the given name, or nil if not found.
func GetClaim(claims []corev1.PersistentVolumeClaim, claimName string) *corev1.PersistentVolumeClaim {
	for i, claim := range claims {
		if claim.Name == claimName {
			return &claims[i]
		}
	}
	return nil
}

// RetrieveActualPVCs returns all existing PVCs for that StatefulSet, per claim name.
func RetrieveActualPVCs(k8sClient k8s.Client, statefulSet appsv1.StatefulSet) (map[string][]corev1.PersistentVolumeClaim, error) {
	pvcs := make(map[string][]corev1.PersistentVolumeClaim)
	for _, podName := range PodNames(statefulSet) {
		for _, claim := range statefulSet.Spec.VolumeClaimTemplates {
			if claim.Name == "" {
				continue
			}
			pvcName := fmt.Sprintf("%s-%s", claim.Name, podName)
			var pvc corev1.PersistentVolumeClaim
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: statefulSet.Namespace, Name: pvcName}, &pvc); err != nil {
				if apierrors.IsNotFound(err) {
					continue // PVC does not exist (yet)
				}
				return nil, err
			}
			if _, exists := pvcs[claim.Name]; !exists {
				pvcs[claim.Name] = make([]corev1.PersistentVolumeClaim, 0)
			}
			pvcs[claim.Name] = append(pvcs[claim.Name], pvc)
		}
	}
	return pvcs, nil
}

// PodName returns the name of the pod with the given ordinal for this StatefulSet.
func PodName(ssetName string, ordinal int32) string {
	return fmt.Sprintf("%s-%d", ssetName, ordinal)
}

// PodNames returns the names of the pods for this StatefulSet, according to the number of replicas.
func PodNames(sset appsv1.StatefulSet) []string {
	names := make([]string, 0, GetReplicas(sset))
	for i := int32(0); i < GetReplicas(sset); i++ {
		names = append(names, PodName(sset.Name, i))
	}
	return names
}

// GetActualPodsForStatefulSet returns the existing pods associated to this StatefulSet.
// The returned pods may not match the expected StatefulSet replicas in a transient situation.
func GetActualPodsForStatefulSet(c k8s.Client, sset types.NamespacedName) ([]corev1.Pod, error) {
	var pods corev1.PodList
	ns := client.InNamespace(sset.Namespace)
	matchLabels := client.MatchingLabels(map[string]string{
		labels.StatefulSetNameLabelName: sset.Name,
	})
	if err := c.List(context.Background(), &pods, matchLabels, ns); err != nil {
		return nil, err
	}
	return pods.Items, nil
}
