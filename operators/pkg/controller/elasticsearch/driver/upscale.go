// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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
		actual, exists := actualStatefulSets.GetByName(ssetToApply.Name)
		if exists && sset.Replicas(ssetToApply) < sset.Replicas(actual) {
			// sset needs to be scaled down
			// update the sset to use the new spec but don't scale replicas down for now
			ssetToApply.Spec.Replicas = actual.Spec.Replicas
		}
		if err := sset.ReconcileStatefulSet(k8sClient, scheme, es, ssetToApply); err != nil {
			return err
		}
	}
	return nil
}
