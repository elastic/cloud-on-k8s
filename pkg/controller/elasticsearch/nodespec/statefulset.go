// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// HeadlessServiceName returns the name of the headless service for the given StatefulSet.
func HeadlessServiceName(ssetName string) string {
	// just use the sset name
	return ssetName
}

// HeadlessService returns a headless service for the given StatefulSet
func HeadlessService(es types.NamespacedName, ssetName string) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      HeadlessServiceName(ssetName),
			Labels:    label.NewStatefulSetLabels(es, ssetName),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Selector:  label.NewStatefulSetLabels(es, ssetName),
		},
	}
}

func BuildStatefulSet(
	es v1beta1.Elasticsearch,
	nodeSet v1beta1.NodeSet,
	cfg settings.CanonicalConfig,
	keystoreResources *keystore.Resources,
	scheme *runtime.Scheme,
) (appsv1.StatefulSet, error) {
	statefulSetName := name.StatefulSet(es.Name, nodeSet.Name)

	// ssetSelector is used to match the sset pods
	ssetSelector := label.NewStatefulSetLabels(k8s.ExtractNamespacedName(&es), statefulSetName)

	// add default PVCs to the node spec
	nodeSet.VolumeClaimTemplates = defaults.AppendDefaultPVCs(
		nodeSet.VolumeClaimTemplates, nodeSet.PodTemplate.Spec, esvolume.DefaultVolumeClaimTemplates...,
	)
	// build pod template
	podTemplate, err := BuildPodTemplateSpec(es, nodeSet, cfg, keystoreResources)
	if err != nil {
		return appsv1.StatefulSet{}, err
	}

	// build sset labels on top of the selector
	// TODO: inherit user-provided labels and annotations from the CRD?
	ssetLabels := make(map[string]string)
	for k, v := range ssetSelector {
		ssetLabels[k] = v
	}

	// set the owner reference of all volume claims to the ES resource,
	// so PVC get deleted automatically upon Elasticsearch resource deletion
	claims := make([]corev1.PersistentVolumeClaim, 0, len(nodeSet.VolumeClaimTemplates))
	for _, claim := range nodeSet.VolumeClaimTemplates {
		if err := controllerutil.SetControllerReference(&es, &claim, scheme); err != nil {
			return appsv1.StatefulSet{}, err
		}
		claims = append(claims, claim)
	}

	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      statefulSetName,
			Labels:    ssetLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
			// we don't care much about pods creation ordering, and manage deletion ordering ourselves,
			// so we're fine with the StatefulSet controller spawning all pods in parallel
			PodManagementPolicy: appsv1.ParallelPodManagement,
			// use default revision history limit
			RevisionHistoryLimit: nil,
			// build a headless service per StatefulSet, matching the StatefulSet labels
			ServiceName: HeadlessServiceName(statefulSetName),
			Selector: &metav1.LabelSelector{
				MatchLabels: ssetSelector,
			},

			Replicas:             &nodeSet.Count,
			VolumeClaimTemplates: claims,
			Template:             podTemplate,
		},
	}

	// store a hash of the sset resource in its labels for comparison purposes
	sset.Labels = hash.SetTemplateHashLabel(sset.Labels, sset.Spec)

	return sset, nil
}

// UpdateReplicas updates the given StatefulSet with the given replicas,
// and modifies the template hash label accordingly.
func UpdateReplicas(statefulSet *appsv1.StatefulSet, replicas *int32) {
	statefulSet.Spec.Replicas = replicas
	statefulSet.Labels = hash.SetTemplateHashLabel(statefulSet.Labels, statefulSet.Spec)
}
