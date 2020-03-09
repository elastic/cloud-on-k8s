// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"strings"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// AssociatedUserType is used to annotate an associated user secret.
	AssociatedUserType = "user"

	// UserNameField is the field in the secret that contains the username.
	UserNameField = "name"
	// PasswordHashField is the field in the secret that contains the hash of the password.
	PasswordHashField = "passwordHash"
	// UserRolesField is the field in the secret that contains the roles for the user as a comma separated list of strings.
	UserRolesField = "userRoles"

	fieldNotFound = "field %s not found in secret %s/%s"
)

// AssociatedUser represents an Elasticsearch user associated for another resource (eg. Kibana, APMServer, etc.).
type AssociatedUser struct {
	Name         string
	PasswordHash []byte
	Roles        []string
}

// AssociatedUserLabels returns labels matching associated users for the given es resource.
func AssociatedUserLabels(es esv1.Elasticsearch) map[string]string {
	return map[string]string{
		label.ClusterNameLabelName: es.Name,
		common.TypeLabelName:       AssociatedUserType,
	}
}

// retrieveAssociatedUsers fetches users resulting from an association (eg. Kibana or APMServer users).
func retrieveAssociatedUsers(c k8s.Client, es esv1.Elasticsearch) (users, error) {
	// list all associated users secret
	var associatedUserSecrets corev1.SecretList
	if err := c.List(
		&associatedUserSecrets,
		client.InNamespace(es.Namespace),
		client.MatchingLabels(AssociatedUserLabels(es)),
	); err != nil {
		return nil, err
	}

	// parse secrets content into users
	users := make([]AssociatedUser, 0, len(associatedUserSecrets.Items))
	for _, secret := range associatedUserSecrets.Items {
		u, err := parseAssociatedUserSecret(secret)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return fromAssociatedUsers(users), nil
}

// parseAssociatedUserSecret reads an associated user from a secret.
func parseAssociatedUserSecret(secret v1.Secret) (AssociatedUser, error) {
	user := AssociatedUser{}
	if len(secret.Data) == 0 {
		return user, pkgerrors.Errorf("user secret %s/%s is empty", secret.Namespace, secret.Name)
	}

	if username, ok := secret.Data[UserNameField]; ok && len(username) > 0 {
		user.Name = string(username)
	} else {
		return user, pkgerrors.Errorf(fieldNotFound, UserNameField, secret.Namespace, secret.Name)
	}

	if hash, ok := secret.Data[PasswordHashField]; ok && len(hash) > 0 {
		user.PasswordHash = hash
	} else {
		return user, pkgerrors.Errorf(fieldNotFound, PasswordHashField, secret.Namespace, secret.Name)
	}

	if roles, ok := secret.Data[UserRolesField]; ok && len(roles) > 0 {
		user.Roles = strings.Split(string(roles), ",")
	}

	return user, nil
}
