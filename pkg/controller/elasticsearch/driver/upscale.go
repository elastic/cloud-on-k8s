// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// HandleUpscaleAndSpecChanges reconciles expected NodeSpec resources.
// It does:
// - create any new StatefulSets
// - update existing StatefulSets specification, to be used for future pods rotation
// - upscale StatefulSet for which we expect more replicas
// It does not:
// - perform any StatefulSet downscale (left for downscale phase)
// - perform any pod upgrade (left for rolling upgrade phase)
func HandleUpscaleAndSpecChanges(
	k8sClient k8s.Client,
	es v1alpha1.Elasticsearch,
	scheme *runtime.Scheme,
	expectedResources nodespec.ResourcesList,
	actualStatefulSets sset.StatefulSetList,
) error {
	// TODO: there is a split brain possibility here if going from 1 to 3 masters or 3 to 7.
	//  We should add only one master node at a time for safety.
	//  See https://github.com/elastic/cloud-on-k8s/issues/1281.

	for _, nodeSpecRes := range expectedResources {
		// always reconcile config (will apply to new & recreated pods)
		if err := settings.ReconcileConfig(k8sClient, es, nodeSpecRes.StatefulSet.Name, nodeSpecRes.Config); err != nil {
			return err
		}
		if _, err := common.ReconcileService(k8sClient, scheme, &nodeSpecRes.HeadlessService, &es); err != nil {
			return err
		}
		ssetToApply := *nodeSpecRes.StatefulSet.DeepCopy()
		actual, alreadyExists := actualStatefulSets.GetByName(ssetToApply.Name)
		if alreadyExists {
			ssetToApply = adaptForExistingStatefulSet(actual, ssetToApply)
		}
		if err := sset.ReconcileStatefulSet(k8sClient, scheme, es, ssetToApply); err != nil {
			return err
		}
	}
	return nil
}

// adaptForExistingStatefulSet modifies ssetToApply to account for the existing StatefulSet.
// It avoids triggering downscales (done later), and makes sure new pods are created with the newest revision.
func adaptForExistingStatefulSet(actualSset appsv1.StatefulSet, ssetToApply appsv1.StatefulSet) appsv1.StatefulSet {
	if sset.GetReplicas(ssetToApply) < sset.GetReplicas(actualSset) {
		// This is a downscale.
		// We still want to update the sset spec to the newest one, but don't scale replicas down for now.
		ssetToApply.Spec.Replicas = actualSset.Spec.Replicas
	}
	// Make sure new pods (with ordinal>partition) get created with the newest revision,
	// by setting the rollingUpdate partition to the actual StatefulSet replicas count.
	// Any ongoing rolling upgrade may temporarily pause here, but will go through again.
	ssetToApply.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: actualSset.Spec.Replicas,
	}
	return ssetToApply
}
