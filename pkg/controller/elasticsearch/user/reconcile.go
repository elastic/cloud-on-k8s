// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"reflect"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// ReconcileUsersAndRoles fetches all users and roles and aggregates them into a single
// Kubernetes secret mounted in the Elasticsearch Pods.
// That secret contains the file realm files (`users` and `users_roles`) and the file roles (`roles.yml`).
// Users are aggregated from various sources:
// - predefined users include the controller user, the probe user, and the public-facing elastic user
// - associated users come from resource associations (eg. Kibana or APMServer)
// - user-provided users from file realms referenced in the Elasticsearch spec
// Roles are aggregated from:
// - predefined roles (for the probe user)
// - user-provided roles referenced in the Elasticsearch spec
func ReconcileUsersAndRoles(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	watched watches.DynamicWatches,
	recorder record.EventRecorder,
	passwordHasher cryptutil.PasswordHasher,
) (esclient.BasicAuth, error) {
	span, ctx := apm.StartSpan(ctx, "reconcile_users", tracing.SpanTypeApp)
	defer span.End()

	// build aggregate roles and file realms
	roles, err := aggregateRoles(ctx, c, es, watched, recorder)
	if err != nil {
		return esclient.BasicAuth{}, err
	}
	fileRealm, controllerUser, err := aggregateFileRealm(ctx, c, es, watched, recorder, passwordHasher)
	if err != nil {
		return esclient.BasicAuth{}, err
	}

	// reconcile the service accounts
	saTokens, err := GetServiceAccountTokens(c, es)
	if err != nil {
		return esclient.BasicAuth{}, err
	}

	// reconcile the aggregate secret
	if err := reconcileRolesFileRealmSecret(ctx, c, es, roles, fileRealm, saTokens); err != nil {
		return esclient.BasicAuth{}, err
	}

	// return the controller user for next reconciliation steps to interact with Elasticsearch
	return controllerUser, nil
}

func getExistingFileRealm(c k8s.Client, es esv1.Elasticsearch) (filerealm.Realm, error) {
	var secret corev1.Secret
	if err := c.Get(context.Background(), RolesFileRealmSecretKey(es), &secret); err != nil {
		return filerealm.Realm{}, err
	}
	return filerealm.FromSecret(secret)
}

// aggregateFileRealm builds a single file realm from multiple ones, and returns the controller user credentials.
func aggregateFileRealm(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	watched watches.DynamicWatches,
	recorder record.EventRecorder,
	passwordHasher cryptutil.PasswordHasher,
) (filerealm.Realm, esclient.BasicAuth, error) {
	// retrieve existing file realm to reuse predefined users password hashes if possible
	existingFileRealm, err := getExistingFileRealm(c, es)
	if err != nil && apierrors.IsNotFound(err) {
		// no secret yet, work with an empty file realm
		existingFileRealm = filerealm.New()
	} else if err != nil {
		return filerealm.Realm{}, esclient.BasicAuth{}, err
	}

	// watch & fetch user-provided file realm & roles
	userProvidedFileRealm, err := reconcileUserProvidedFileRealm(ctx, c, es, existingFileRealm, watched, recorder, passwordHasher)
	if err != nil {
		return filerealm.Realm{}, esclient.BasicAuth{}, err
	}

	// reconcile predefined users
	elasticUser, err := reconcileElasticUser(ctx, c, es, existingFileRealm, userProvidedFileRealm, passwordHasher)
	if err != nil {
		return filerealm.Realm{}, esclient.BasicAuth{}, err
	}
	internalUsers, err := reconcileInternalUsers(ctx, c, es, existingFileRealm, passwordHasher)
	if err != nil {
		return filerealm.Realm{}, esclient.BasicAuth{}, err
	}

	// fetch associated users
	associatedUsers, err := retrieveAssociatedUsers(c, es)
	if err != nil {
		return filerealm.Realm{}, esclient.BasicAuth{}, err
	}

	// merge all file realms together, the last one having precedence
	fileRealm := filerealm.MergedFrom(
		internalUsers.fileRealm(),
		elasticUser.fileRealm(),
		associatedUsers.fileRealm(),
		userProvidedFileRealm,
	)

	// grab the controller user credentials for later use
	controllerCreds, err := internalUsers.credentialsFor(ControllerUserName)
	if err != nil {
		return filerealm.Realm{}, esclient.BasicAuth{}, err
	}
	return fileRealm, controllerCreds, nil
}

func aggregateRoles(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	watched watches.DynamicWatches,
	recorder record.EventRecorder,
) (RolesFileContent, error) {
	userProvided, err := reconcileUserProvidedRoles(ctx, c, es, watched, recorder)
	if err != nil {
		return RolesFileContent{}, err
	}
	// merge all roles together, the last one having precedence
	return PredefinedRoles.MergeWith(userProvided), nil
}

// RolesFileRealmSecretKey returns a reference to the K8s secret holding the roles and file realm data.
func RolesFileRealmSecretKey(es esv1.Elasticsearch) types.NamespacedName {
	return types.NamespacedName{Namespace: es.Namespace, Name: esv1.RolesAndFileRealmSecret(es.Name)}
}

// reconcileRolesFileRealmSecret creates or updates the single secret holding the file realm and the file-based roles.
func reconcileRolesFileRealmSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	roles RolesFileContent,
	fileRealm filerealm.Realm,
	saTokens ServiceAccountTokens,
) error {
	secretData := fileRealm.FileBytes()
	rolesBytes, err := roles.FileBytes()
	if err != nil {
		return err
	}
	secretData[RolesFile] = rolesBytes
	secretData[ServiceTokensFileName] = saTokens.ToBytes()

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: RolesFileRealmSecretKey(es).Namespace,
			Name:      RolesFileRealmSecretKey(es).Name,
			Labels:    label.NewLabels(k8s.ExtractNamespacedName(&es)),
		},
		Data: secretData,
	}
	// TODO: factorize with https://github.com/elastic/cloud-on-k8s/issues/2626
	var reconciled corev1.Secret
	return reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			// update if secret content is different
			return !reflect.DeepEqual(expected.Data, reconciled.Data) ||
				// or expected labels are not there
				!maps.IsSubset(expected.Labels, reconciled.Labels)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data
			maps.Merge(reconciled.Labels, expected.Labels)
			maps.Merge(reconciled.Annotations, expected.Annotations)
		},
	})
}
