// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"

	"gopkg.in/yaml.v2"
)

const (
	// RolesFile is the name of the roles file in the ES config dir.
	RolesFile = "roles.yml"

	// SuperUserBuiltinRole is the name of the built-in superuser role.
	SuperUserBuiltinRole = "superuser"
	// ProbeUserRole is the name of the role used by the internal probe user.
	ProbeUserRole = "elastic_internal_probe_user"
)

var (
	// PredefinedRoles to create for internal needs.
	PredefinedRoles = RolesFileContent{
		ProbeUserRole: esclient.Role{Cluster: []string{"monitor"}},
	}
)

// RolesFileContent is a map {role name -> yaml role spec}.
// We care about the role names here, but consider the roles spec as a yaml blob we don't need to access.
type RolesFileContent map[string]interface{}

// parseRolesFileContent returns a RolesFileContent from the given data.
// Since rolesFileContent already corresponds to a deserialized yaml representation of the roles files,
// we just unmarshal from the yaml data.
func parseRolesFileContent(data []byte) (RolesFileContent, error) {
	var parsed RolesFileContent
	err := yaml.Unmarshal(data, &parsed)
	return parsed, err
}

// fileBytes returns the file representation of rolesFileContent.
// Since rolesFileContent already corresponds to a deserialized yaml representation of the roles files,
// we just marshal it back to yaml.
func (r RolesFileContent) FileBytes() ([]byte, error) {
	return yaml.Marshal(&r)
}

// mergeWith merges multiple rolesFileContent, giving priority to other.
func (r RolesFileContent) MergeWith(other RolesFileContent) RolesFileContent {
	for roleName, roleSpec := range other {
		r[roleName] = roleSpec
	}
	return r
}
