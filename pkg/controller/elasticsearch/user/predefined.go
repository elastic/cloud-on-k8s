// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	// ElasticUserName is the public-facing user.
	ElasticUserName = "elastic"
	// ControllerUserName is the controller user to interact with ES.
	ControllerUserName = "elastic-internal"
	// MonitoringUserName is used for the Elasticsearch monitoring.
	MonitoringUserName = "elastic-internal-monitoring"
	// PreStopUserName is used for API interactions from the pre-stop Pod lifecycle hook
	PreStopUserName = "elastic-internal-pre-stop"
	// ProbeUserName is used for the Elasticsearch readiness probe.
	ProbeUserName = "elastic-internal-probe"
	// DiagnosticsUserName is used for the ECK diagnostics.
	DiagnosticsUserName = "elastic-internal-diagnostics"
)

// reconcileElasticUser reconciles a single secret holding the "elastic" user password.
func reconcileElasticUser(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	existingFileRealm,
	userProvidedFileRealm filerealm.Realm,
	passwordHasher cryptutil.PasswordHasher,
) (users, error) {
	if es.Spec.Auth.DisableElasticUser {
		return nil, nil
	}
	secretName := esv1.ElasticUserSecret(es.Name)
	// if user has set up the elastic user via the file realm do not create the operator managed secret to avoid confusion
	if userProvidedFileRealm.PasswordHashForUser(ElasticUserName) != nil {
		return nil, k8s.DeleteSecretIfExists(ctx, c, types.NamespacedName{
			Namespace: es.Namespace,
			Name:      secretName,
		})
	}
	// regular reconciliation if user did not choose to set a password for the elastic user
	return reconcilePredefinedUsers(
		ctx,
		c,
		es,
		existingFileRealm,
		users{
			{Name: ElasticUserName, Roles: []string{SuperUserBuiltinRole}},
		},
		secretName,
		// Don't set an ownerRef for the elastic user secret, likely to be copied into different namespaces.
		// See https://github.com/elastic/cloud-on-k8s/issues/3986.
		false,
		passwordHasher,
	)
}

// reconcileInternalUsers reconciles a single secret holding the internal users passwords.
func reconcileInternalUsers(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	existingFileRealm filerealm.Realm,
	passwordHasher cryptutil.PasswordHasher,
) (users, error) {
	users := users{
		{Name: ControllerUserName, Roles: []string{SuperUserBuiltinRole}},
		{Name: PreStopUserName, Roles: []string{ClusterManageRole}},
		{Name: ProbeUserName, Roles: []string{ProbeUserRole}},
		{Name: MonitoringUserName, Roles: []string{RemoteMonitoringCollectorBuiltinRole}},
		{Name: DiagnosticsUserName, Roles: []string{DiagnosticsUserRoleV85}},
	}
	ver, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, fmt.Errorf("while parsing Elasticsearch version (%s): %w", es.Spec.Version, err)
	}
	if ver.LT(version.From(8, 5, 0)) {
		// Diagnostics user needs Superuser role in 7.x.
		if err := setRolesForUser(DiagnosticsUserName, users, []string{SuperUserBuiltinRole}); err != nil {
			return nil, err
		}
		// If 8.0.0 >= version < 8.5.0, the Diagnostics user needs the DiagnosticsUserRoleV80 role.
		if ver.GTE(version.From(8, 0, 0)) {
			if err := setRolesForUser(DiagnosticsUserName, users, []string{DiagnosticsUserRoleV80}); err != nil {
				return nil, err
			}
		}
	}
	return reconcilePredefinedUsers(
		ctx,
		c,
		es,
		existingFileRealm,
		users,
		esv1.InternalUsersSecret(es.Name),
		true,
		passwordHasher,
	)
}

// setRolesForUser sets the roles for the given user given a slice of users.
// It returns an error if the user is not found.
func setRolesForUser(userName string, users []user, roles []string) error {
	for i, user := range users {
		if user.Name == userName {
			users[i].Roles = roles
			return nil
		}
	}
	return fmt.Errorf("user %s not found", userName)
}

// reconcilePredefinedUsers reconciles a secret with the given name holding the given users.
// It attempts to reuse passwords from pre-existing secrets, and reuse hashes from pre-existing file realms.
func reconcilePredefinedUsers(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	existingFileRealm filerealm.Realm,
	users users,
	secretName string,
	setOwnerRef bool,
	passwordHasher cryptutil.PasswordHasher,
) (users, error) {
	secretNsn := types.NamespacedName{Namespace: es.Namespace, Name: secretName}

	// build users, reusing existing passwords and bcrypt hashes if possible
	var err error
	users, err = reuseOrGeneratePassword(c, users, secretNsn)
	if err != nil {
		return nil, err
	}
	users, err = reuseOrGenerateHashes(users, existingFileRealm, passwordHasher)
	if err != nil {
		return nil, err
	}

	// reconcile secret
	secretData := make(map[string][]byte, len(users))
	for _, u := range users {
		secretData[u.Name] = u.Password
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNsn.Namespace,
			Name:      secretNsn.Name,
			Labels:    labels.AddCredentialsLabel(label.NewLabels(k8s.ExtractNamespacedName(&es))),
		},
		Data: secretData,
	}

	if setOwnerRef {
		_, err = reconciler.ReconcileSecret(ctx, c, expected, &es)
	} else {
		_, err = reconciler.ReconcileSecretNoOwnerRef(ctx, c, expected, &es)
	}
	return users, err
}

// reuseOrGeneratePassword updates the users with existing passwords reused from the existing K8s secret,
// or generates new passwords.
func reuseOrGeneratePassword(c k8s.Client, users users, secretRef types.NamespacedName) (users, error) {
	var secret corev1.Secret
	err := c.Get(context.Background(), secretRef, &secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	// default to an empty secret
	if apierrors.IsNotFound(err) {
		secret = corev1.Secret{}
	}
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	// either reuse the password or generate a new one
	for i, u := range users {
		if password, exists := secret.Data[u.Name]; exists {
			users[i].Password = password
		} else {
			users[i].Password = common.FixedLengthRandomPasswordBytes()
		}
	}
	return users, nil
}

// reuseOrGenerateHashes updates the users with existing hashes from the given file realm, or generates new ones.
func reuseOrGenerateHashes(users users, fileRealm filerealm.Realm, passwordHasher cryptutil.PasswordHasher) (users, error) {
	for i, u := range users {
		existingHash := fileRealm.PasswordHashForUser(u.Name)
		hash, err := passwordHasher.ReuseOrGenerateHash(u.Password, existingHash)
		if err != nil {
			return nil, err
		}
		users[i].PasswordHash = hash
	}
	return users, nil
}

func GetMonitoringUserPassword(c k8s.Client, nsn types.NamespacedName) (string, error) {
	secretObjKey := types.NamespacedName{Namespace: nsn.Namespace, Name: esv1.InternalUsersSecret(nsn.Name)}
	var secret corev1.Secret
	if err := c.Get(context.Background(), secretObjKey, &secret); err != nil {
		return "", err
	}

	passwordBytes, ok := secret.Data[MonitoringUserName]
	if !ok {
		return "", errors.Errorf("auth secret key %s doesn't exist", MonitoringUserName)
	}

	return string(passwordBytes), nil
}
