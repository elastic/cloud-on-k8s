// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	assoctype "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoteClusterSpec defines the desired state of RemoteCluster.
type RemoteClusterSpec struct {
	Remote InClusterSpec `json:"remote"`
}

type InClusterSpec struct {
	InRemoteCluster assoctype.ObjectSelector `json:"inCluster"`
}

// RemoteClusterStatus defines the observed state of RemoteCluster.
type RemoteClusterStatus struct {
	State                  string          `json:"state,omitempty"`
	ClusterName            string          `json:"clusterName,omitempty"`
	LocalTrustRelationship string          `json:"localTrustRelationship,omitempty"`
	SeedHosts              []string        `json:"seedHosts,omitempty"`
	InClusterStatus        InClusterStatus `json:"inCluster,omitempty"`
}

// InClusterStatus defines the state of the inCluster driver state.
type InClusterStatus struct {
	RemoteSelector          assoctype.ObjectSelector `json:"remoteSelector,omitempty"`
	RemoteTrustRelationship string                   `json:"remoteTrustRelationship,omitempty"`
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
