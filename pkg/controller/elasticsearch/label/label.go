// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package label

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

const (
	// ClusterNameLabelName used to represent a cluster in k8s resources
	ClusterNameLabelName = "elasticsearch.k8s.elastic.co/cluster-name"
	// ClusterNamespaceLabelName used to represent a cluster in k8s resources
	ClusterNamespaceLabelName = "elasticsearch.k8s.elastic.co/cluster-namespace"
	// VersionLabelName used to store the Elasticsearch version of the resource
	VersionLabelName = "elasticsearch.k8s.elastic.co/version"
	// PodNameLabelName used to store the name of the pod on other objects
	PodNameLabelName = "elasticsearch.k8s.elastic.co/pod-name"
	// StatefulSetNameLabelName used to store the name of the statefulset.
	StatefulSetNameLabelName = "elasticsearch.k8s.elastic.co/statefulset-name"
	// NodeTypesMasterLabelName is a label set to true on nodes with the master role
	NodeTypesMasterLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-master"
	// NodeTypesDataLabelName is a label set to true on nodes with the data role
	NodeTypesDataLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data"
	// NodeTypesIngestLabelName is a label set to true on nodes with the ingest role
	NodeTypesIngestLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ingest"
	// NodeTypesMLLabelName is a label set to true on nodes with the ml role
	NodeTypesMLLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ml"
	// NodeTypesTransformLabelName is a label set to true on nodes with the transform role
	NodeTypesTransformLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-transform"
	// NodeTypesRemoteClusterClientLabelName is a label set to true on nodes with the remote_cluster_client role
	NodeTypesRemoteClusterClientLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-remote_cluster_client"
	// NodeTypesVotingOnlyLabelName is a label set to true on voting-only master-eligible nodes
	NodeTypesVotingOnlyLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-voting_only"
	// NodeTypesDataColdLabelName is a label set to true on nodes with the data_cold role.
	NodeTypesDataColdLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data_cold"
	// NodeTypesDataContentLabelName is a label set to true on nodes with the data_content role.
	NodeTypesDataContentLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data_content"
	// NodeTypesDataHotLabelName is a label set to true on nodes with the data_hot role.
	NodeTypesDataHotLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data_hot"
	// NodeTypesDataWarmLabelName is a label set to true on nodes with the data_warm role.
	NodeTypesDataWarmLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data_warm"
	// NodeTypesDataFrozenLabelName is a label set to true on nodes with the data_frozen role.
	NodeTypesDataFrozenLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data_frozen"

	HTTPSchemeLabelName = "elasticsearch.k8s.elastic.co/http-scheme"

	// Type represents the Elasticsearch type
	Type = "elasticsearch"
)

// NonMasterRoles are all Elasticsearch node roles except master or voting-only.
var NonMasterRoles = []common.TrueFalseLabel{
	NodeTypesDataLabelName,
	NodeTypesDataHotLabelName,
	NodeTypesDataColdLabelName,
	NodeTypesDataFrozenLabelName,
	NodeTypesDataContentLabelName,
	NodeTypesDataWarmLabelName,
	NodeTypesIngestLabelName,
	NodeTypesMLLabelName,
	NodeTypesRemoteClusterClientLabelName,
	NodeTypesTransformLabelName,
}

// IsMasterNode returns true if the pod has the master node label
func IsMasterNode(pod corev1.Pod) bool {
	return NodeTypesMasterLabelName.HasValue(true, pod.Labels)
}

// IsMasterNodeSet returns true if the given StatefulSet specifies master nodes.
func IsMasterNodeSet(statefulSet appsv1.StatefulSet) bool {
	return NodeTypesMasterLabelName.HasValue(true, statefulSet.Spec.Template.Labels)
}

// IsDataNodeSet returns true if the given StatefulSet specifies data nodes.
func IsDataNodeSet(statefulSet appsv1.StatefulSet) bool {
	return NodeTypesDataLabelName.HasValue(true, statefulSet.Spec.Template.Labels)
}

// IsIngestNodeSet returns true if the given StatefulSet specifies ingest nodes.
func IsIngestNodeSet(statefulSet appsv1.StatefulSet) bool {
	return NodeTypesIngestLabelName.HasValue(true, statefulSet.Spec.Template.Labels)
}

func FilterMasterNodePods(pods []corev1.Pod) []corev1.Pod {
	masters := []corev1.Pod{}
	for _, pod := range pods {
		if IsMasterNode(pod) {
			masters = append(masters, pod)
		}
	}
	return masters
}

// IsDataNode returns true if the pod has the data node label
func IsDataNode(pod corev1.Pod) bool {
	return NodeTypesDataLabelName.HasValue(true, pod.Labels)
}

// ExtractVersion extracts the Elasticsearch version from the given labels.
func ExtractVersion(labels map[string]string) (version.Version, error) {
	return version.FromLabels(labels, VersionLabelName)
}

// NewLabels constructs a new set of labels from an Elasticsearch definition.
func NewLabels(es types.NamespacedName) map[string]string {
	return map[string]string{
		ClusterNameLabelName: es.Name,
		common.TypeLabelName: Type,
	}
}

// NewPodLabels returns labels to apply for a new Elasticsearch pod.
func NewPodLabels(
	es types.NamespacedName,
	ssetName string,
	ver version.Version,
	nodeRoles *esv1.Node,
	scheme string,
) map[string]string {
	// cluster name based labels
	labels := NewLabels(es)
	// version label
	labels[VersionLabelName] = ver.String()

	// node types labels
	NodeTypesMasterLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.MasterRole), labels)
	NodeTypesDataLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.DataRole), labels)
	NodeTypesIngestLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.IngestRole), labels)
	NodeTypesMLLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.MLRole), labels)
	// transform and remote_cluster_client roles were only added in 7.7.0 so we should not annotate previous versions with them
	if ver.GTE(version.From(7, 7, 0)) {
		NodeTypesTransformLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.TransformRole), labels)
		NodeTypesRemoteClusterClientLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.RemoteClusterClientRole), labels)
	}
	// voting_only master eligible nodes were added only in 7.3.0 so we don't want to label prior versions with it
	if ver.GTE(version.From(7, 3, 0)) {
		NodeTypesVotingOnlyLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.VotingOnlyRole), labels)
	}
	// data tiers were added in 7.10.0
	if ver.GTE(version.From(7, 10, 0)) {
		NodeTypesDataContentLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.DataContentRole), labels)
		NodeTypesDataColdLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.DataColdRole), labels)
		NodeTypesDataHotLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.DataHotRole), labels)
		NodeTypesDataWarmLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.DataWarmRole), labels)
	}
	// frozen tier has been introduced in 7.12.0
	if ver.GTE(version.From(7, 12, 0)) {
		NodeTypesDataFrozenLabelName.Set(nodeRoles.IsConfiguredWithRole(esv1.DataFrozenRole), labels)
	}

	labels[HTTPSchemeLabelName] = scheme

	// apply stateful set label selector
	for k, v := range NewStatefulSetLabels(es, ssetName) {
		labels[k] = v
	}

	return labels
}

// NewConfigLabels returns labels to apply for an Elasticsearch Config secret.
func NewConfigLabels(es types.NamespacedName, ssetName string) map[string]string {
	return NewStatefulSetLabels(es, ssetName)
}

func NewStatefulSetLabels(es types.NamespacedName, ssetName string) map[string]string {
	lbls := NewLabels(es)
	lbls[StatefulSetNameLabelName] = ssetName
	return lbls
}

// NewLabelSelectorForElasticsearch returns a labels.Selector that matches the labels as constructed by NewLabels
func NewLabelSelectorForElasticsearch(es esv1.Elasticsearch) client.MatchingLabels {
	return NewLabelSelectorForElasticsearchClusterName(es.Name)
}

// NewLabelSelectorForElasticsearchClusterName returns a labels.Selector that matches the labels as constructed by
// NewLabels for the provided cluster name.
func NewLabelSelectorForElasticsearchClusterName(clusterName string) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{ClusterNameLabelName: clusterName})
}

// NewLabelSelectorForStatefulSetName returns a labels.Selector that matches the labels set on resources managed for
// a given StatefulSet in a cluster.
func NewLabelSelectorForStatefulSetName(clusterName, ssetName string) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{
		ClusterNameLabelName:     clusterName,
		StatefulSetNameLabelName: ssetName,
	})
}

// ClusterFromResourceLabels returns the NamespacedName of the Elasticsearch associated
// to the given resource, by retrieving its name from the resource labels.
// It does implicitly consider the cluster and the resource to be in the same namespace.
func ClusterFromResourceLabels(metaObject metav1.Object) (types.NamespacedName, bool) {
	resourceName, exists := metaObject.GetLabels()[ClusterNameLabelName]
	return types.NamespacedName{
		Namespace: metaObject.GetNamespace(),
		Name:      resourceName,
	}, exists
}
