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

// usersRoles is a map {role name -> [] sorted user names}
// /!\ we use the File Realm naming convention, note this is a role to users mapping,
// and not the other way around as the name may indicate.
type usersRoles map[string][]string

// mergeWith merges multiple usersRoles, giving priority to other.
func (r usersRoles) mergeWith(other usersRoles) usersRoles {
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
		// role already exists, merge sorted users and remove duplicates
		userSet := set.Make(currentUsers...)
		userSet.MergeWith(set.Make(otherUsers...))
		userSlice := userSet.AsSlice()
		stringsutil.SortStringSlice(userSlice)
		r[otherRole] = userSlice
	}
	return r
}

// fileBytes serializes the usersRoles into a file with format:
// ```
// role1:user1,user2,user3
// role2:user1
// ```
// Rows are sorted for easier comparison.
func (r usersRoles) fileBytes() []byte {
	rows := make([]string, 0, len(r))
	for role, users := range r {
		rows = append(rows, fmt.Sprintf("%s:%s", role, strings.Join(users, ",")))
	}
	// sort rows for consistent comparison
	// users within each row are already sorted
	stringsutil.SortStringSlice(rows)
	return []byte(strings.Join(rows, "\n") + "\n")
}

// parseUsersRoles extracts the role to users mapping from the given file content.
// Expected format:
// ```
// role1:user1,user2,user3
// role2:user1
// ```
func parseUsersRoles(data []byte) (usersRoles, error) {
	rolesMapping := make(usersRoles)
	return rolesMapping, forEachRow(data, func(row []byte) error {
		roleUsers := strings.Split(string(row), ":")
		if len(roleUsers) != 2 {
			return fmt.Errorf("invalid entry in users_roles")
		}
		role := roleUsers[0]
		var users []string
		if len(roleUsers[1]) > 0 {
			users = strings.Split(roleUsers[1], ",")
			// sort users for consistent comparison
			stringsutil.SortStringSlice(users)
		}
		rolesMapping = rolesMapping.mergeWith(usersRoles{
			role: users,
		})
		return nil
	})
}
