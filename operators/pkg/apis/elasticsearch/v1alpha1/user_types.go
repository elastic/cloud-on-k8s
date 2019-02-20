// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserSpec defines the desired state of User.
type UserSpec struct {
	Name        string                   `json:"name"`
	PasswordRef corev1.SecretKeySelector `json:"passwordRef"`
	UserRoles   []string                 `json:"userRoles"`
	// We don't need custom roles right now and we would need an adapter layer anyway to translate into the
	// version specific representations of any role spec'ed here for Elasticsearch
	// roles       []RoleSpec         `json:"roles"`
}

// UserStatus defines the observed state of User
type UserStatus struct {
	Phase  UserPhase `json:"phase,omitempty"`
	Reason string    `json:"reason,omitempty"`
}

// UserPhase is the phase in the lifecycle of a user resource
type UserPhase string

const (
	// UserPending means user resource has not been created in Elasticsearch yet.
	UserPending UserPhase = "pending"
	// UserCreated means user has been created as defined in this resource in Elasticsearch.
	UserCreated UserPhase = "created"
	// UserInvalid means this user resource was invalid and could not be created in Elasticsearch.
	UserInvalid UserPhase = "invalid"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// User is the Schema for the users API
// +k8s:openapi-gen=true
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec,omitempty"`
	Status UserStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UserList contains a list of User
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}
