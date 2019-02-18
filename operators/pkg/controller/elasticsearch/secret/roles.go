// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package secret

import "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"

const (
	// SuperUserBuiltinRole is the name of the built-in superuser role
	SuperUserBuiltinRole = "superuser"
	// KibanaUserBuiltinRole is the name of the built-in kibana_user role
	KibanaUserBuiltinRole = "kibana_user"
	// ProbeUserRole is the name of the custom probe_user role
	ProbeUserRole = "probe_user"
)

// InternalRoles are roles used by internal users
var InternalRoles = map[string]client.Role{
	ProbeUserRole: {
		Cluster: []string{"monitor"},
	},
}