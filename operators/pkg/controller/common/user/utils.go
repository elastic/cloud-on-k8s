// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

// GetSecret gets the first secret in a list that matches the namespace and the name.
func GetSecret(list corev1.SecretList, namespace, name string) *corev1.Secret {
	for _, secret := range list.Items {
		if secret.Namespace == namespace && secret.Name == name {
			return &secret
		}
	}
	return nil
}

// ChecksUser checks that a secret contains the required fields expected by the user reconciler.
func ChecksUser(t *testing.T, secret *corev1.Secret, expectedUsername string, expectedRoles []string) {
	assert.NotNil(t, secret)
	currentUsername, ok := secret.Data["name"]
	assert.True(t, ok)
	assert.Equal(t, expectedUsername, string(currentUsername))
	passwordHash, ok := secret.Data["passwordHash"]
	assert.True(t, ok)
	assert.NotEmpty(t, passwordHash)
	currentRoles, ok := secret.Data["userRoles"]
	assert.True(t, ok)
	assert.ElementsMatch(t, expectedRoles, strings.Split(string(currentRoles), ","))
}
