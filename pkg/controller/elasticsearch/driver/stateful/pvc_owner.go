// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// reconcilePVCOwnerRefs removes legacy ECK ES ownerReferences from PVCs: PVC lifecycle is
// delegated to the upstream StatefulSet PersistentVolumeClaimRetentionPolicy.
// Returns the names of the PVCs still waiting for the upstream StatefulSet controller to stamp
// its ownerRef before the ES ref can be removed; the caller should requeue if any.
func reconcilePVCOwnerRefs(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, actualStatefulSets es_sset.StatefulSetList) ([]string, error) {
	var pvcs corev1.PersistentVolumeClaimList
	ns := client.InNamespace(es.Namespace)
	labelSelector := label.NewLabelSelectorForElasticsearch(es)
	if err := c.List(ctx, &pvcs, ns, labelSelector); err != nil {
		return nil, fmt.Errorf("while listing pvcs to reconcile owner refs: %w", err)
	}

	policy := es.Spec.VolumeClaimDeletePolicyOrDefault()
	// inRangePVCNames is the set of PVC names currently managed by a live StatefulSet pod.
	// Used under DeleteOnScaledownAndClusterDeletionPolicy to avoid waiting indefinitely for
	// out-of-range PVCs (pre-upgrade scale-down leftovers) that will never receive an upstream ref.
	inRangePVCNames := sets.New(actualStatefulSets.PVCNames()...)

	// ssetNames is used to only consider ownerRefs pointing to this cluster's StatefulSets.
	ssetNames := actualStatefulSets.Names()

	var waitingForUpstream []string
	for _, pvc := range pvcs.Items {
		needsUpdate := false

		switch policy {
		case esv1.DeleteOnScaledownAndClusterDeletionPolicy:
			esRefIdx, hasUpstreamRef := inspectOwnerRefs(&pvc, &es, ssetNames)
			if esRefIdx < 0 {
				break
			}

			// Remove the legacy ES ownerRef only once the upstream StatefulSet controller has
			// stamped its own ref: StatefulSet for in-range pods, Pod for condemned pods
			// mid scale-down. Waiting avoids leaving PVCs unowned during the first
			// post-upgrade reconcile before the upstream controller has had a chance to run.
			if hasUpstreamRef {
				pvc.OwnerReferences = slices.Delete(pvc.OwnerReferences, esRefIdx, esRefIdx+1)
				needsUpdate = true
				break
			}

			// Before #4050, SetControllerReference stamped the ES ref with Controller=true.
			// Kubernetes treats any foreign controller ref as unexpected and will not stamp its
			// own StatefulSet/Pod ref while it is present. Downgrade to a non-controller ref
			// so the upstream controller can take over; we remove the ES ref on the next reconcile.
			if esRef := &pvc.OwnerReferences[esRefIdx]; ptr.Deref(esRef.Controller, false) {
				esRef.Controller = new(false)
				needsUpdate = true
			}

			if inRangePVCNames.Has(pvc.Name) {
				// In-range: wait for upstream to stamp its own ref before removing the ES ref.
				waitingForUpstream = append(waitingForUpstream, pvc.Name)
			}
			// Out-of-range: leave the PVC untouched and do not wait. The ES ref keeps this
			// PVC deletable at cluster deletion. If the nodeSet later scales back up, the pod
			// is created, upstream stamps its ref, and the in-range path above removes the ES
			// ref at that point.
		case esv1.DeleteOnScaledownOnlyPolicy:
			// Strip the ES ownerRef: unowned is the correct steady state for this policy since
			// PVCs are intended to survive cluster deletion. Also strip stale StatefulSet ownerRefs
			// left from a previous DeleteOnScaledownAndClusterDeletion policy.
			refs := pvc.GetOwnerReferences()
			filtered := slices.DeleteFunc(refs, func(ref metav1.OwnerReference) bool {
				return isESOwnerRef(ref, &es) || isClusterStatefulSetOwnerRef(ref, ssetNames)
			})
			if len(filtered) != len(refs) {
				pvc.SetOwnerReferences(filtered)
				needsUpdate = true
			}
		}

		if !needsUpdate {
			continue
		}
		if err := c.Update(ctx, &pvc); err != nil {
			return nil, fmt.Errorf("while updating pvc during owner ref reconciliation: %w", err)
		}
	}
	if len(waitingForUpstream) > 0 {
		ulog.FromContext(ctx).V(1).Info("Waiting for the StatefulSet controller to set PVC ownerRefs",
			"namespace", es.Namespace, "es_name", es.Name, "pvc_names", waitingForUpstream)
	}
	return waitingForUpstream, nil
}

// setPVCOwnerRefsForRecreation adds a non-controller ES ownerRef to the PVCs of the StatefulSets scheduled for
// re-creation under DeleteOnScaledownAndClusterDeletionPolicy. The StatefulSet is deleted with orphan propagation,
// which strips its ownerRef from the PVCs: without the ES ref, the PVCs would be orphaned if the cluster is deleted
// before the StatefulSet is re-created. reconcilePVCOwnerRefs removes the ES ref once the re-created StatefulSet
// has stamped its own.
func setPVCOwnerRefsForRecreation(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) error {
	if es.Spec.VolumeClaimDeletePolicyOrDefault() != esv1.DeleteOnScaledownAndClusterDeletionPolicy {
		return nil
	}
	toRecreate, err := volume.StatefulSetsToRecreate(&es)
	if err != nil {
		return err
	}
	for _, pvcName := range es_sset.StatefulSetList(toRecreate).PVCNames() {
		var pvc corev1.PersistentVolumeClaim
		if err := c.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: pvcName}, &pvc); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("while getting pvc to set owner ref before StatefulSet re-creation: %w", err)
		}
		if k8s.HasOwner(&pvc, &es) {
			continue
		}
		// Non-controller ref: a controller ref would prevent the upstream StatefulSet controller from stamping its own.
		if err := controllerutil.SetOwnerReference(&es, &pvc, scheme.Scheme); err != nil {
			return fmt.Errorf("while setting owner ref before StatefulSet re-creation: %w", err)
		}
		if err := c.Update(ctx, &pvc); err != nil {
			return fmt.Errorf("while updating pvc to set owner ref before StatefulSet re-creation: %w", err)
		}
	}
	return nil
}

// inspectOwnerRefs walks the PVC ownerReferences and returns the index of the ES ownerRef
// (-1 if absent) and whether an ownerRef stamped by the upstream StatefulSet controller is present.
func inspectOwnerRefs(pvc *corev1.PersistentVolumeClaim, es *esv1.Elasticsearch, ssetNames set.StringSet) (esRefIdx int, hasUpstreamRef bool) {
	esRefIdx = -1
	for i, ref := range pvc.OwnerReferences {
		switch {
		case isESOwnerRef(ref, es):
			esRefIdx = i
		case isClusterStatefulSetOwnerRef(ref, ssetNames), isOwnPodOwnerRef(pvc, ref):
			hasUpstreamRef = true
		}
	}
	return esRefIdx, hasUpstreamRef
}

// isESOwnerRef reports whether the ownerReference points to the given Elasticsearch resource.
func isESOwnerRef(ref metav1.OwnerReference, es *esv1.Elasticsearch) bool {
	return ref.Name == es.Name && ref.UID == es.UID
}

// isClusterStatefulSetOwnerRef reports whether the ownerReference points to one of the cluster's StatefulSets,
// as stamped by the upstream StatefulSet controller for in-range pods.
func isClusterStatefulSetOwnerRef(ref metav1.OwnerReference, ssetNames set.StringSet) bool {
	return ref.APIVersion == appsv1.SchemeGroupVersion.String() && ref.Kind == "StatefulSet" && ssetNames.Has(ref.Name)
}

// isOwnPodOwnerRef reports whether the ownerReference points to the Pod using the PVC, as stamped by the
// upstream StatefulSet controller for condemned pods mid scale-down. PVCs are named {claim}-{pod}.
func isOwnPodOwnerRef(pvc *corev1.PersistentVolumeClaim, ref metav1.OwnerReference) bool {
	return ref.APIVersion == corev1.SchemeGroupVersion.String() && ref.Kind == "Pod" && strings.HasSuffix(pvc.Name, "-"+ref.Name)
}
