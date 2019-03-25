// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ObjectSelector allows to specify a reference to an object across namespace boundaries.
type ObjectSelector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// NamespacedName is a convenience method to turn an ObjectSelector into a NamespaceName.
func (s ObjectSelector) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Name:      s.Name,
		Namespace: s.Namespace,
	}
}

// RemoteClusterSpec defines the desired state of RemoteCluster.
type RemoteClusterSpec struct {
	Remote InClusterSpec `json:"remote"`
}

type InClusterSpec struct {
	InRemoteCluster ObjectSelector `json:"inCluster"`
}

// RemoteClusterStatus defines the observed state of RemoteCluster.
type RemoteClusterStatus struct {
	State                                string         `json:"state,omitempty"`
	ClusterName                          string         `json:"cluster-name,omitempty"`
	LocalTrustRelationshipName           string         `json:"localTrustRelationshipName,omitempty"`
	InClusterRemoteSelector              ObjectSelector `json:"inClusterRemoteSelector,omitempty"`
	InClusterRemoteTrustRelationshipName string         `json:"inClusterRemoteTrustRelationshipName,omitempty"`
	SeedHosts                            []string       `json:"seedHosts,omitempty"`
}

const (
	RemoteClusterPropagated      string = "Propagated"
	RemoteClusterFailed          string = "Failed"
	RemoteClusterRemovalFailed   string = "RemovalFailed"
	RemoteClusterPending         string = "Pending"
	RemoteClusterDeletionPending string = "DeletionPending"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RemoteCluster is the Schema for the remoteclusters API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type RemoteCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteClusterSpec   `json:"spec,omitempty"`
	Status RemoteClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RemoteClusterList contains a list of RemoteCluster
type RemoteClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteCluster{}, &RemoteClusterList{})
}
