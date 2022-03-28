// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// reconcilePVCOwnerRefs sets or removes an owner reference into each PVC for the given Elasticsearch cluster depending
// on the VolumeClaimDeletePolicy.
// The intent behind this approach is to allow users to specify per cluster whether they want to retain or remove
// the related PVCs. We rely on Kubernetes garbage collection for the cleanup once a cluster has been deleted and
// the operator separately deletes PVCs on scale down if so desired (see GarbageCollectPVCs)
func reconcilePVCOwnerRefs(c k8s.Client, es esv1.Elasticsearch) error {
	var pvcs corev1.PersistentVolumeClaimList
	ns := client.InNamespace(es.Namespace)
	labelSelector := label.NewLabelSelectorForElasticsearch(es)
	if err := c.List(context.Background(), &pvcs, ns, labelSelector); err != nil {
		return fmt.Errorf("while listing pvcs to reconcile owner refs: %w", err)
	}

	for _, pvc := range pvcs.Items {
		pvc := pvc
		hasOwner := k8s.HasOwner(&pvc, &es)
		switch es.Spec.VolumeClaimDeletePolicyOrDefault() {
		case esv1.DeleteOnScaledownOnlyPolicy:
			if !hasOwner {
				continue
			}
			k8s.RemoveOwner(&pvc, &es)
		case esv1.DeleteOnScaledownAndClusterDeletionPolicy:
			if hasOwner {
				continue
			}
			if err := controllerutil.SetOwnerReference(&es, &pvc, scheme.Scheme); err != nil {
				return fmt.Errorf("while setting owner during owner ref reconciliation: %w", err)
			}
		}
		if err := c.Update(context.Background(), &pvc); err != nil {
			return fmt.Errorf("while updating pvc during owner ref reconciliation: %w", err)
		}
	}
	return nil
}
