// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_reconcileElasticUser(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	tests := []struct {
		name              string
		existingSecrets   []client.Object
		existingFileRealm filerealm.Realm
		assertions        func(t *testing.T, u users)
	}{
		{
			name:              "create a new elastic user if it does not exist yet",
			existingSecrets:   nil,
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// a random password should be generated
				require.NotEmpty(t, u[0].Password)
			},
		},
		{
			name: "elastic user secret exists but is invalid: generate a new elastic user",
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       nil, // no password or password removed
				},
			},
			existingFileRealm: filerealm.New().WithUser(ElasticUserName, []byte("$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6")),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// password should be regenerated
				require.NotEmpty(t, u[0].Password)
				// hash should be regenerated
				require.NotEmpty(t, u[0].PasswordHash)
				require.NotEqual(t, "$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6", u[0].PasswordHash)
			},
		},
		{
			name: "reuse the existing elastic user and password hash",
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       map[string][]byte{ElasticUserName: []byte("existingPassword")},
				},
			},
			existingFileRealm: filerealm.New().WithUser(ElasticUserName, []byte("$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6")),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// password and hashes should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6"), u[0].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if the existing one doesn't match",
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       map[string][]byte{ElasticUserName: []byte("existingPassword")},
				},
			},
			existingFileRealm: filerealm.New().WithUser(ElasticUserName, []byte("does-not-match-password")),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// password should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				// hash should be re-computed
				require.NotEqual(t, []byte("does-not-match-password"), u[0].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if there is none in the file realm",
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       map[string][]byte{ElasticUserName: []byte("existingPassword")},
				},
			},
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// password should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				// hash should be computed
				require.NotEmpty(t, u[0].PasswordHash)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.existingSecrets...)
			got, err := reconcileElasticUser(context.Background(), c, es, tt.existingFileRealm, filerealm.New(), testPasswordHasher)
			require.NoError(t, err)
			// check returned user
			require.Len(t, got, 1)
			user := got[0]
			// name and roles are always the same
			require.Equal(t, ElasticUserName, user.Name)
			require.Equal(t, []string{SuperUserBuiltinRole}, user.Roles)
			// password and hash should always match
			require.NoError(t, bcrypt.CompareHashAndPassword(user.PasswordHash, user.Password))
			// reconciled secret should have the updated password
			var secret corev1.Secret
			err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)}, &secret)
			require.NoError(t, err)
			require.Equal(t, user.Password, secret.Data[ElasticUserName])
			tt.assertions(t, got)
		})
	}
}

func Test_reconcileElasticUser_conditionalCreation(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	tests := []struct {
		name         string
		userFileReam filerealm.Realm
		wantUser     bool
	}{
		{
			name:     "create a new elastic user if it is not in the user's file realm",
			wantUser: true,
		},
		{
			name:         "do not create the elastic user secret if the elastic user is already defined by the user",
			userFileReam: filerealm.New().WithUser(ElasticUserName, []byte("some-hash")),
			wantUser:     false,
		},
		{
			name:         "do create the elastic user if other non-empty file realm users are defined by user",
			userFileReam: filerealm.New().WithUser("other", []byte("some-hash")),
			wantUser:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient()
			got, err := reconcileElasticUser(context.Background(), c, es, filerealm.New(), tt.userFileReam, testPasswordHasher)
			require.NoError(t, err)
			// check returned user
			wantLen := 1
			if !tt.wantUser {
				wantLen = 0
			}
			require.Len(t, got, wantLen)
		})
	}
}

func Test_reconcileInternalUsers(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}, Spec: esv1.ElasticsearchSpec{Version: "8.10.0"}}
	tests := []struct {
		name              string
		es                func() esv1.Elasticsearch
		existingSecrets   []client.Object
		existingFileRealm filerealm.Realm
		assertions        func(t *testing.T, u users)
		errorExpected     bool
	}{
		{
			name:              "create new internal users if they do not exist yet",
			es:                func() esv1.Elasticsearch { return es },
			existingSecrets:   nil,
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// random passwords should be generated
				require.NotEmpty(t, u[0].Password)
				require.NotEmpty(t, u[1].Password)
			},
		},
		{
			name: "reuse the existing passwords and hashes",
			es:   func() esv1.Elasticsearch { return es },
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.InternalUsersSecret(es.Name)},
					Data: map[string][]byte{
						ControllerUserName: []byte("controllerUserPassword"),
						ProbeUserName:      []byte("probeUserPassword"),
					},
				},
			},
			existingFileRealm: filerealm.New().
				WithUser(ControllerUserName, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG")).
				WithUser(ProbeUserName, []byte("$2a$10$8.9my2W7FVDqDnh.E1RwouN5RzkZGulQ3ZMgmoy3CH4xRvr5uYPbS")),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// passwords and hashes should be reused
				require.Equal(t, []byte("controllerUserPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG"), u[0].PasswordHash)
				require.Equal(t, []byte("probeUserPassword"), u[2].Password)
				require.Equal(t, []byte("$2a$10$8.9my2W7FVDqDnh.E1RwouN5RzkZGulQ3ZMgmoy3CH4xRvr5uYPbS"), u[2].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if the existing one doesn't match",
			es:   func() esv1.Elasticsearch { return es },
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.InternalUsersSecret(es.Name)},
					Data: map[string][]byte{
						ControllerUserName: []byte("controllerUserPassword"),
						ProbeUserName:      []byte("probeUserPassword"),
					},
				},
			},
			existingFileRealm: filerealm.New().
				WithUser(ControllerUserName, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG")).
				WithUser(ProbeUserName, []byte("does-not-match-password")),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// password & hash of controller user should be reused
				require.Equal(t, []byte("controllerUserPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG"), u[0].PasswordHash)
				// password of probe user should be reused, but hash should be re-computed
				require.Equal(t, []byte("probeUserPassword"), u[2].Password)
				require.NotEmpty(t, u[1].PasswordHash)
				require.NotEqual(t, "does-not-match-password", u[2].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if there is none in the file realm",
			es:   func() esv1.Elasticsearch { return es },
			existingSecrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.InternalUsersSecret(es.Name)},
					Data: map[string][]byte{
						ControllerUserName: []byte("controllerUserPassword"),
						ProbeUserName:      []byte("probeUserPassword"),
					},
				},
			},
			existingFileRealm: filerealm.New().
				// missing probe user hash
				WithUser(ControllerUserName, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG")),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				// password & hash of controller user should be reused
				require.Equal(t, []byte("controllerUserPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG"), u[0].PasswordHash)
				// password of probe user should be reused, and hash should be re-computed
				require.Equal(t, []byte("probeUserPassword"), u[2].Password)
				require.NotEmpty(t, u[2].PasswordHash)
			},
		},
		{
			name: "ES 7.x - diagnostic user uses superuser role",
			es: func() esv1.Elasticsearch {
				es := es.DeepCopy()
				es.Spec.Version = "7.10.0"
				return *es
			},
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				require.Len(t, u, 5)
				require.Equal(t, []string{SuperUserBuiltinRole}, u[4].Roles)
			},
		},
		{
			name: "ES 8.4 - diagnostic user uses specific 'DiagnosticsUserRoleV80' role",
			es: func() esv1.Elasticsearch {
				es := es.DeepCopy()
				es.Spec.Version = "8.4.0"
				return *es
			},
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				t.Helper()
				require.Len(t, u, 5)
				require.Equal(t, []string{DiagnosticsUserRoleV80}, u[4].Roles)
			},
		},
		{
			name: "Invalid ES version returns error",
			es: func() esv1.Elasticsearch {
				es := es.DeepCopy()
				es.Spec.Version = "invalid"
				return *es
			},
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				t.Helper()
			},
			errorExpected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.existingSecrets...)
			got, err := reconcileInternalUsers(context.Background(), c, tt.es(), tt.existingFileRealm, testPasswordHasher)
			require.True(t, ((err != nil) == tt.errorExpected), "error expected: %v, got: %v", tt.errorExpected, err)
			if tt.errorExpected {
				return
			}
			// check returned users
			require.Len(t, got, 5)
			controllerUser := got[0]
			probeUser := got[2]
			// names and roles are always the same
			require.Equal(t, ControllerUserName, controllerUser.Name)
			require.Equal(t, []string{SuperUserBuiltinRole}, controllerUser.Roles)
			require.Equal(t, ProbeUserName, probeUser.Name)
			require.Equal(t, []string{ProbeUserRole}, probeUser.Roles)
			// passwords and hash should always match
			require.NoError(t, bcrypt.CompareHashAndPassword(controllerUser.PasswordHash, controllerUser.Password))
			require.NoError(t, bcrypt.CompareHashAndPassword(probeUser.PasswordHash, probeUser.Password))
			// reconciled secret should have the updated passwords
			var secret corev1.Secret
			err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.InternalUsersSecret(es.Name)}, &secret)
			require.NoError(t, err)
			require.Equal(t, controllerUser.Password, secret.Data[ControllerUserName])
			require.Equal(t, probeUser.Password, secret.Data[ProbeUserName])
			tt.assertions(t, got)
		})
	}
}
