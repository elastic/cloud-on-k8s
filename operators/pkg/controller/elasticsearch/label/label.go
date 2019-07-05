// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	// StatefulSetNameLabelName used to store the name of the statefulset
	StatefulSetNameLabelName = "elasticsearch.k8s.elastic.co/statefulset"
	// VolumeNameLabelName is the name of the volume e.g. elasticsearch-data a PVC was used for.
	VolumeNameLabelName = "elasticsearch.k8s.elastic.co/volume-name"

	// NodeTypesMasterLabelName is a label set to true on nodes with the master role
	NodeTypesMasterLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-master"
	// NodeTypesDataLabelName is a label set to true on nodes with the data role
	NodeTypesDataLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data"
	// NodeTypesIngestLabelName is a label set to true on nodes with the ingest role
	NodeTypesIngestLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ingest"
	// NodeTypesMLLabelName is a label set to true on nodes with the ml role
	NodeTypesMLLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ml"

	// Type represents the Elasticsearch type
	Type = "elasticsearch"
)

// IsMasterNode returns true if the pod has the master node label
func IsMasterNode(pod corev1.Pod) bool {
	return NodeTypesMasterLabelName.HasValue(true, pod.Labels)
}

// IsDataNode returns true if the pod has the data node label
func IsDataNode(pod corev1.Pod) bool {
	return NodeTypesDataLabelName.HasValue(true, pod.Labels)
}

// ExtractVersion extracts the Elasticsearch version from a pod label.
func ExtractVersion(pod corev1.Pod) (*version.Version, error) {
	labelValue, ok := pod.Labels[VersionLabelName]
	if !ok {
		return nil, fmt.Errorf("pod %s is missing the version label %s", pod.Name, VersionLabelName)
	}
	v, err := version.Parse(labelValue)
	if err != nil {
		return nil, errors.Wrapf(err, "pod %s has an invalid version label", pod.Name)
	}
	return v, nil
}

// NewLabels constructs a new set of labels from an Elasticsearch definition.
func NewLabels(es types.NamespacedName) map[string]string {
	return map[string]string{
		ClusterNameLabelName: es.Name,
		common.TypeLabelName: Type,
	}
}

// NewPodLabels returns labels to apply for a new Elasticsearch pod.
func NewPodLabels(es v1alpha1.Elasticsearch, version version.Version, cfg v1alpha1.ElasticsearchSettings) map[string]string {
	// cluster name based labels
	labels := NewLabels(k8s.ExtractNamespacedName(&es))
	// version label
	labels[VersionLabelName] = version.String()
	// node types labels
	NodeTypesMasterLabelName.Set(cfg.Node.Master, labels)
	NodeTypesDataLabelName.Set(cfg.Node.Data, labels)
	NodeTypesIngestLabelName.Set(cfg.Node.Ingest, labels)
	NodeTypesMLLabelName.Set(cfg.Node.ML, labels)

	return labels
}

// NewLabelSelectorForElasticsearch returns a labels.Selector that matches the labels as constructed by NewLabels
func NewLabelSelectorForElasticsearch(es v1alpha1.Elasticsearch) labels.Selector {
	return NewLabelSelectorForElasticsearchClusterName(es.Name)
}

// NewLabelSelectorForElasticsearchClusterName returns a labels.Selector that matches the labels as constructed by
// NewLabels for the provided cluster name.
func NewLabelSelectorForElasticsearchClusterName(clusterName string) labels.Selector {
	return labels.SelectorFromSet(map[string]string{ClusterNameLabelName: clusterName})
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
