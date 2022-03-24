// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestReconcileUsersAndRoles(t *testing.T) {
	c := k8s.NewFakeClient(append(sampleUserProvidedFileRealmSecrets, sampleUserProvidedRolesSecret...)...)
	controllerUser, err := ReconcileUsersAndRoles(context.Background(), c, sampleEsWithAuth, initDynamicWatches(), record.NewFakeRecorder(10))
	require.NoError(t, err)
	require.NotEmpty(t, controllerUser.Password)
	var reconciledSecret corev1.Secret
	err = c.Get(context.Background(), RolesFileRealmSecretKey(sampleEsWithAuth), &reconciledSecret)
	require.NoError(t, err)
	require.Len(t, reconciledSecret.Data, 4)
	require.NotEmpty(t, reconciledSecret.Data[RolesFile])
	require.NotEmpty(t, reconciledSecret.Data[filerealm.UsersRolesFile])
	require.NotEmpty(t, reconciledSecret.Data[filerealm.UsersFile])
}

func Test_ReconcileRolesFileRealmSecret(t *testing.T) {
	c := k8s.NewFakeClient()
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	roles := RolesFileContent{"click_admins": []byte(`run_as: [ 'clicks_watcher_1' ]
  cluster: [ 'monitor' ]
  indices:
  - names: [ 'events-*' ]
    privileges: [ 'read' ]
    field_security:
      grant: ['category', '@timestamp', 'message' ]
    query: '{"match": {"category": "click"}}'`)}
	realm := filerealm.New().
		WithUser("user1", []byte("hash1")).
		WithUser("user2", []byte("hash2")).
		WithRole("role1", []string{"user1"}).
		WithRole("role2", []string{"user2"})

	saTokens := ServiceAccountTokens{}.
		Add(ServiceAccountToken{
			FullyQualifiedServiceAccountName: "fqsa2",
			HashedSecret:                     "hash2",
		}).
		Add(ServiceAccountToken{
			FullyQualifiedServiceAccountName: "fqsa1",
			HashedSecret:                     "hash1",
		})
	err := reconcileRolesFileRealmSecret(c, es, roles, realm, saTokens)
	require.NoError(t, err)
	// retrieve reconciled secret
	var secret corev1.Secret
	err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.RolesAndFileRealmSecret(es.Name)}, &secret)
	require.NoError(t, err)
	require.Len(t, secret.Data, 4)
	require.Contains(t, string(secret.Data[RolesFile]), "click_admins")
	require.Contains(t, string(secret.Data[filerealm.UsersRolesFile]), "role1:user1")
	require.Contains(t, string(secret.Data[filerealm.UsersFile]), "user1:hash1")
	require.Equal(t, string(secret.Data[ServiceTokensFileName]), "fqsa1:hash1\nfqsa2:hash2\n")
}

func Test_aggregateFileRealm(t *testing.T) {
	c := k8s.NewFakeClient(sampleUserProvidedFileRealmSecrets...)
	fileRealm, controllerUser, err := aggregateFileRealm(c, sampleEsWithAuth, initDynamicWatches(), record.NewFakeRecorder(10))
	require.NoError(t, err)
	require.NotEmpty(t, controllerUser.Password)
	actualUsers := fileRealm.UserNames()
	require.ElementsMatch(t, []string{"elastic", "elastic-internal", "elastic-internal-probe", "elastic-internal-monitoring", "user1", "user2", "user3"}, actualUsers)
}

func Test_aggregateRoles(t *testing.T) {
	c := k8s.NewFakeClient(sampleUserProvidedRolesSecret...)
	roles, err := aggregateRoles(c, sampleEsWithAuth, initDynamicWatches(), record.NewFakeRecorder(10))
	require.NoError(t, err)
	require.Len(t, roles, 51)
	require.Contains(t, roles, ProbeUserRole, "role1", "role2")
}
