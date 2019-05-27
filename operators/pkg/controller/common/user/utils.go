package user

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func GetSecret(list corev1.SecretList, namespace, name string) *corev1.Secret {
	for _, secret := range list.Items {
		if secret.Namespace == namespace && secret.Name == name {
			return &secret
		}
	}
	return nil
}

// CheckEsUser checks that a secret contains the required fields expected by the user reconciler.
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
