// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	esvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// HeadlessServiceName returns the name of the headless service for the given StatefulSet.
func HeadlessServiceName(ssetName string) string {
	// just use the sset name
	return ssetName
}

// HeadlessService returns a headless service for the given StatefulSet
func HeadlessService(es *esv1.Elasticsearch, ssetName string) corev1.Service {
	nsn := k8s.ExtractNamespacedName(es)

	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      HeadlessServiceName(ssetName),
			Labels:    label.NewStatefulSetLabels(nsn, ssetName),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Selector:  label.NewStatefulSetLabels(nsn, ssetName),
			Ports: []corev1.ServicePort{
				{
					Name:     es.Spec.HTTP.Protocol(),
					Protocol: corev1.ProtocolTCP,
					Port:     network.HTTPPort,
				},
			},
			// allow nodes to discover themselves via DNS while they are booting up ie. are not ready yet
			PublishNotReadyAddresses: true,
		},
	}
}

func BuildStatefulSet(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	nodeSet esv1.NodeSet,
	cfg settings.CanonicalConfig,
	keystoreResources *keystore.Resources,
	existingStatefulSets es_sset.StatefulSetList,
	setDefaultSecurityContext bool,
	policyConfig PolicyConfig,
) (appsv1.StatefulSet, error) {
	statefulSetName := esv1.StatefulSet(es.Name, nodeSet.Name)

	// ssetSelector is used to match the sset pods
	ssetSelector := label.NewStatefulSetLabels(k8s.ExtractNamespacedName(&es), statefulSetName)

	// add default PVCs to the node spec only if no user defined PVCs exist
	nodeSet.VolumeClaimTemplates = defaults.AppendDefaultPVCs(
		nodeSet.VolumeClaimTemplates,
		nodeSet.PodTemplate.Spec,
		esvolume.DefaultVolumeClaimTemplates...,
	)

	// build pod template
	podTemplate, err := BuildPodTemplateSpec(ctx, client, es, nodeSet, cfg, keystoreResources, setDefaultSecurityContext, policyConfig)
	if err != nil {
		return appsv1.StatefulSet{}, err
	}

	// build sset labels on top of the selector
	// TODO: inherit user-provided labels and annotations from the CRD?
	ssetLabels := make(map[string]string)
	for k, v := range ssetSelector {
		ssetLabels[k] = v
	}

	// maybe inherit volumeClaimTemplates ownerRefs from the existing StatefulSet
	var existingClaims []corev1.PersistentVolumeClaim
	if existingSset, exists := existingStatefulSets.GetByName(statefulSetName); exists {
		existingClaims = existingSset.Spec.VolumeClaimTemplates
	}
	claims := preserveExistingVolumeClaimsOwnerRefs(nodeSet.VolumeClaimTemplates, existingClaims)

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
			PodManagementPolicy:  appsv1.ParallelPodManagement,
			RevisionHistoryLimit: es.Spec.RevisionHistoryLimit,
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

func preserveExistingVolumeClaimsOwnerRefs(
	persistentVolumeClaims []corev1.PersistentVolumeClaim,
	existingClaims []corev1.PersistentVolumeClaim,
) []corev1.PersistentVolumeClaim {
	// before https://github.com/elastic/cloud-on-k8s/pull/4050, we used to set an ownerRef into all claims
	// now keep existing ownerReferences for backwards compatibility but don't add new ones
	claims := make([]corev1.PersistentVolumeClaim, 0, len(persistentVolumeClaims))
	for _, claim := range persistentVolumeClaims {
		if existingClaim := sset.GetClaim(existingClaims, claim.Name); existingClaim != nil {
			// This claim already exists in the actual resource. Since the volumeClaimTemplates section of
			// a StatefulSet is immutable, any modification to it will be rejected in the StatefulSet update.
			// This is fine and we let it error-out. It is caught in a user-friendly way by the validating webhook.
			//
			// However, there is one case where the claim we build may differ from the existing one, that was
			// built with a prior version of the operator. If the Elasticsearch apiVersion has changed,
			// from eg. `v1beta1` to `v1`, we want to keep the existing ownerRef (pointing to eg. a `v1beta1` owner).
			// Having ownerReferences with a "deprecated" apiVersion is fine, and does not prevent resources
			// from being garbage collected as expected.
			claim.OwnerReferences = existingClaim.OwnerReferences
		}
		claims = append(claims, claim)
	}
	return claims
}

// UpdateReplicas updates the given StatefulSet with the given replicas,
// and modifies the template hash label accordingly.
func UpdateReplicas(statefulSet *appsv1.StatefulSet, replicas *int32) {
	statefulSet.Spec.Replicas = replicas
	statefulSet.Labels = hash.SetTemplateHashLabel(statefulSet.Labels, statefulSet.Spec)
}
