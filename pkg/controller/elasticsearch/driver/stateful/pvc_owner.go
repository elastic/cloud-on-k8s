// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ReconcilePVCOwnerRefs sets or removes an ownerReference on each PVC for the given Elasticsearch cluster
// depending on VolumeClaimDeletePolicy. It is also called during the deletion path to stamp any PVC the
// StatefulSet controller may have recreated during the deletion window. We rely on Kubernetes GC for cleanup
// once the cluster is deleted, and separately delete PVCs on scale-down when desired (see GarbageCollectPVCs).
func ReconcilePVCOwnerRefs(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) error {
	var pvcs corev1.PersistentVolumeClaimList
	ns := client.InNamespace(es.Namespace)
	labelSelector := label.NewLabelSelectorForElasticsearch(es)
	if err := c.List(ctx, &pvcs, ns, labelSelector); err != nil {
		return fmt.Errorf("while listing pvcs to reconcile owner refs: %w", err)
	}

	for _, pvc := range pvcs.Items {
		hasOwner := k8s.HasOwner(&pvc, &es)
		needsUpdate := false

		switch es.Spec.VolumeClaimDeletePolicyOrDefault() {
		case esv1.DeleteOnScaledownOnlyPolicy:
			if hasOwner {
				k8s.RemoveOwner(&pvc, &es)
				needsUpdate = true
			}
			// Remove any stale StatefulSet ownerRefs left over from a previous
			// DeleteOnScaledownAndClusterDeletionPolicy.
			if removeStatefulSetOwnerRefs(&pvc) {
				needsUpdate = true
			}
		case esv1.DeleteOnScaledownAndClusterDeletionPolicy:
			if !hasOwner {
				if err := controllerutil.SetOwnerReference(&es, &pvc, scheme.Scheme); err != nil {
					return fmt.Errorf("while setting owner during owner ref reconciliation: %w", err)
				}
				needsUpdate = true
			}
		}

		if !needsUpdate {
			continue
		}
		if err := c.Update(ctx, &pvc); err != nil {
			return fmt.Errorf("while updating pvc during owner ref reconciliation: %w", err)
		}
	}
	return nil
}

// removeStatefulSetOwnerRefs removes all ownerReferences with Kind=StatefulSet from the PVC
// and reports whether any were removed.
func removeStatefulSetOwnerRefs(pvc *corev1.PersistentVolumeClaim) bool {
	refs := pvc.GetOwnerReferences()
	filtered := slices.DeleteFunc(refs, func(ref metav1.OwnerReference) bool {
		return ref.Kind == "StatefulSet"
	})
	if len(filtered) == len(refs) {
		return false
	}
	pvc.SetOwnerReferences(filtered)
	return true
}
