// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"

const (
	// ExternalUserName also known as the 'elastic'
	ExternalUserName = "elastic"
	// InternalControllerUserName a user to be used from this controller when interacting with ES.
	InternalControllerUserName = "elastic-internal"
	// InternalProbeUserName is a user to be used from the liveness/readiness probes when interacting with ES.
	InternalProbeUserName = "elastic-internal-probe"
	// InternalReloadCredsUserName is a user to be used for reloading ES credentials.
	InternalReloadCredsUserName = "elastic-internal-reload-creds"

	// SuperUserBuiltinRole is the name of the built-in superuser role
	SuperUserBuiltinRole = "superuser"
	// KibanaUserBuiltinRole is the name of the built-in kibana_user role
	KibanaUserBuiltinRole = "kibana_user"
	// ProbeUserRole is the name of the custom probe_user role
	ProbeUserRole = "probe_user"
	// ReloadCredsUserRole is the name of the custom reload_creds_user role
	ReloadCredsUserRole = "reload_creds_user"
)

// Predefined users and roles.
var (
	externalUsers = []User{
		New(ExternalUserName, Roles(SuperUserBuiltinRole)),
	}
	internalUsers = []User{
		New(InternalControllerUserName, Roles(SuperUserBuiltinRole)),
		New(InternalProbeUserName, Roles(ProbeUserRole)),
		New(InternalReloadCredsUserName, Roles(ReloadCredsUserRole)),
	}

	PredefinedRoles = map[string]client.Role{
		ProbeUserRole: {
			Cluster: []string{"monitor"},
		},
		ReloadCredsUserRole: {
			Cluster: []string{"all"},
		},
	}
)

// InternalUsers are Elasticsearch users intended for system use.
type InternalUsers struct {
	ControllerUser  User
	ProbeUser       User
	ReloadCredsUser User
}

func NewInternalUsersFrom(users []User) InternalUsers {
	internalUsers := InternalUsers{}
	for _, user := range users {
		if user.Id() == InternalControllerUserName {
			internalUsers.ControllerUser = user
		}
		if user.Id() == InternalProbeUserName {
			internalUsers.ProbeUser = user
		}
		if user.Id() == InternalReloadCredsUserName {
			internalUsers.ReloadCredsUser = user
		}
	}
	return internalUsers
}
