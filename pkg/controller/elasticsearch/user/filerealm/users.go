// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// usersPasswordHashes is a map {username -> user password hash}
type usersPasswordHashes map[string][]byte

// mergeWith merges multiple usersPasswordHashes, giving priority to other.
func (u usersPasswordHashes) mergeWith(other usersPasswordHashes) usersPasswordHashes {
	if len(u) == 0 {
		return other
	}
	for user, hash := range other {
		u[user] = hash
	}
	return u
}

// fileBytes serializes the usersPasswordHashes into a file with format:
// ```
// username1:passwordHash1
// username2:passwordHash2
// ```
// Rows are sorted for easier comparison.
func (u usersPasswordHashes) fileBytes() []byte {
	rows := make([]string, 0, len(u))
	for user, hash := range u {
		rows = append(rows, fmt.Sprintf("%s:%s", user, hash))
	}
	// sort for consistent comparison
	stringsutil.SortStringSlice(rows)
	return []byte(strings.Join(rows, "\n") + "\n")
}

// parseUsersPasswordHashes extracts users and their password hashes from the given file content.
// Expected format:
// ```
// username1:passwordHash1
// username2:passwordHash2
// ```
func parseUsersPasswordHashes(data []byte) (usersPasswordHashes, error) {
	usersHashes := make(usersPasswordHashes)
	return usersHashes, forEachRow(data, func(row []byte) error {
		userHash := bytes.Split(row, []byte(":"))
		if len(userHash) != 2 {
			return fmt.Errorf("invalid entry in users")
		}
		usersHashes = usersHashes.mergeWith(usersPasswordHashes{
			string(userHash[0]): userHash[1], // user: password hash
		})
		return nil
	})
}
