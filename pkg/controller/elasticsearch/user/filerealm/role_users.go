// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// roleUsersMapping is a map {role name -> [] user names}
type roleUsersMapping map[string][]string

// mergeWith merges multiple usersPasswordHashes, giving priority to other.
func (r roleUsersMapping) mergeWith(other roleUsersMapping) roleUsersMapping {
	if len(r) == 0 {
		return other
	}
	if len(other) == 0 {
		return r
	}
	for otherRole, otherUsers := range other {
		currentUsers, exists := r[otherRole]
		if !exists {
			// role does not exist yet, create it
			r[otherRole] = otherUsers
			continue
		}
		// role already exists, merge sorted users
		userSet := set.Make(currentUsers...)
		userSet.MergeWith(set.Make(otherUsers...))
		userSlice := userSet.AsSlice()
		stringsutil.SortStringSlice(userSlice)
		r[otherRole] = userSlice
	}
	return r
}

// fileBytes serializes the roleUsersMapping into a file with format:
// ```
// role1:user1,user2,user3
// role2:user1
// ```
// Rows are sorted for easier comparison.
func (r roleUsersMapping) fileBytes() []byte {
	rows := make([]string, 0, len(r))
	for role, users := range r {
		stringsutil.SortStringSlice(users)
		rows = append(rows, fmt.Sprintf("%s:%s", role, strings.Join(users, ",")))
	}
	// sort for consistent comparison
	stringsutil.SortStringSlice(rows)
	return []byte(strings.Join(rows, "\n") + "\n")
}

// parseRoleUsersMapping extracts the role to users mapping from the given file content.
// Expected format:
// ```
// role1:user1,user2,user3
// role2:user1
// ```
func parseRoleUsersMapping(data []byte) (roleUsersMapping, error) {
	rolesMapping := make(roleUsersMapping)
	return rolesMapping, forEachRow(data, func(row []byte) error {
		roleUsers := strings.Split(string(row), ":")
		if len(roleUsers) != 2 {
			return fmt.Errorf("invalid entry in users_roles")
		}
		role := roleUsers[0]
		users := strings.Split(roleUsers[1], ",")
		if len(users) == 1 && users[0] == "" {
			// if there are no users, strings.Split("", ",") still returns []string{""}
			// remove that empty user
			users = nil
		}
		rolesMapping = rolesMapping.mergeWith(roleUsersMapping{
			role: users,
		})
		return nil
	})
}
