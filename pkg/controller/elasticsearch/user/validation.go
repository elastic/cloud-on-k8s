// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/errors"
)

const minNameLength = 1
const maxNameLength = 1024
const minPasswordLength = 6

// Validate implements a subset of the elasticsearch-users command's validations
// see https://www.elastic.co/guide/en/elasticsearch/reference/current/users-command.html
func (u user) Validate() error {
	errlist := []error{
		validUserOrRoleName(u.Name),
		validPassword(u.Password),
	}
	for _, role := range u.Roles {
		errlist = append(errlist, validUserOrRoleName(role))
	}
	return errors.NewAggregate(errlist)
}

func validUserOrRoleName(name string) error {
	if len(name) < minNameLength {
		return fmt.Errorf("name %s too short, must be between %d and %d", name, minNameLength, maxNameLength)
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("name %s too long, must be between %d and %d", name, minNameLength, maxNameLength)
	}

	if strings.HasPrefix(name, " ") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("name %s must not start or end with whitespace", name)
	}

	for _, c := range name {
		if c < 32 || c > 126 {
			return fmt.Errorf("name %s invalid, must contain only printable ASCII characters (ASCII 32-126)", name)
		}
	}
	return nil
}

func validPassword(password []byte) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters long", minPasswordLength)
	}
	return nil
}
