// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ClusterNameLabelName used to represent a cluster in k8s resources
	ClusterNameLabelName = "elasticsearch.k8s.elastic.co/cluster-name"
	// VersionLabelName used to store the Elasticsearch version of the resource
	VersionLabelName = "elasticsearch.k8s.elastic.co/version"
	// PodNameLabelName used to store the name of the pod on other objects
	PodNameLabelName = "elasticsearch.k8s.elastic.co/pod-name"
	// StatefulSetNameLabelName used to store the name of the statefulset.
	StatefulSetNameLabelName = "elasticsearch.k8s.elastic.co/statefulset-name"

	// ConfigHashLabelName is a label used to store a hash of the Elasticsearch configuration.
	ConfigHashLabelName = "elasticsearch.k8s.elastic.co/config-hash"
	// SecureSettingsHashLabelName is a label used to store a hash of the Elasticsearch secure settings secret.
	SecureSettingsHashLabelName = "elasticsearch.k8s.elastic.co/secure-settings-hash"

	// NodeTypesMasterLabelName is a label set to true on nodes with the master role
	NodeTypesMasterLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-master"
	// NodeTypesDataLabelName is a label set to true on nodes with the data role
	NodeTypesDataLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data"
	// NodeTypesIngestLabelName is a label set to true on nodes with the ingest role
	NodeTypesIngestLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ingest"
	// NodeTypesMLLabelName is a label set to true on nodes with the ml role
	NodeTypesMLLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ml"

	HTTPSchemeLabelName = "elasticsearch.k8s.elastic.co/http-scheme"

	// Type represents the Elasticsearch type
	Type = "elasticsearch"
)

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
func ExtractVersion(labels map[string]string) (*version.Version, error) {
	return version.FromLabels(labels, VersionLabelName)
}

// MinVersion extracts the currently running Elasticsearch versions from the running pods
func MinVersion(pods []corev1.Pod) (*version.Version, error) {
	vs := make([]version.Version, 0, len(pods))
	for _, pod := range pods {
		v, err := ExtractVersion(pod.Labels)
		if err != nil {
			return nil, err
		}
		vs = append(vs, *v)
	}
	return version.Min(vs), nil
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
	version version.Version,
	nodeRoles esv1.Node,
	configHash string,
	scheme string,
) (map[string]string, error) {
	// cluster name based labels
	labels := NewLabels(es)
	// version label
	labels[VersionLabelName] = version.String()

	// node types labels
	NodeTypesMasterLabelName.Set(nodeRoles.Master, labels)
	NodeTypesDataLabelName.Set(nodeRoles.Data, labels)
	NodeTypesIngestLabelName.Set(nodeRoles.Ingest, labels)
	NodeTypesMLLabelName.Set(nodeRoles.ML, labels)

	// config hash label, to rotate pods on config changes
	labels[ConfigHashLabelName] = configHash

	labels[HTTPSchemeLabelName] = scheme

	// apply stateful set label selector
	for k, v := range NewStatefulSetLabels(es, ssetName) {
		labels[k] = v
	}

	return labels, nil
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

// NewToRequestsFuncFromClusterNameLabel creates a watch handler function that creates reconcile requests based on the
// the cluster name label on the watched resource.
func NewToRequestsFuncFromClusterNameLabel() handler.ToRequestsFunc {
	return handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
		labels := obj.Meta.GetLabels()
		if clusterName, ok := labels[ClusterNameLabelName]; ok {
			// we don't need to special case the handling of this label to support in-place changes to its value
			// as controller-runtime will ask this func to map both the old and the new resources on updates.
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{Namespace: obj.Meta.GetNamespace(), Name: clusterName}},
			}
		}
		return nil
	})
}
