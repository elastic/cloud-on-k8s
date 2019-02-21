// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package secret

import (
	"fmt"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
)

const (
	// ExternalUserName also known as the 'elastic' user.
	ExternalUserName = "elastic"
	// InternalControllerUserName a user to be used from this controller when interacting with ES.
	InternalControllerUserName = "elastic-internal"
	// InternalKibanaServerUserName is a user to be used by the Kibana server when interacting with ES.
	InternalKibanaServerUserName = "elastic-internal-kibana"
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
// Note: The role of a user is not persisted in a k8s secret, that's why ResolveRole(username) exists.
var (
	externalUsers = []client.User{
		{Name: ExternalUserName, Role: SuperUserBuiltinRole},
	}
	internalUsers = []client.User{
		{Name: InternalControllerUserName, Role: SuperUserBuiltinRole},
		{Name: InternalKibanaServerUserName, Role: KibanaUserBuiltinRole},
		{Name: InternalProbeUserName, Role: ProbeUserRole},
		{Name: InternalReloadCredsUserName, Role: ReloadCredsUserRole},
	}
	predefinedUsers = append(internalUsers, externalUsers...)

	PredefinedRoles = map[string]client.Role{
		ProbeUserRole: {
			Cluster: []string{"monitor"},
		},
		ReloadCredsUserRole: {
			Cluster: []string{"all"},
		},
	}
)

// ResolveRole try to find the role of a user by searching in the predefined users.
func ResolveRole(username string) (string, error) {
	for _, predefinedUser := range predefinedUsers {
		if username == predefinedUser.Name && predefinedUser.Role != "" {
			return predefinedUser.Role, nil
		}
	}
	return "", fmt.Errorf("cannot resolve role for user `%s`", username)
}
