// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_reconcileElasticUser(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	tests := []struct {
		name              string
		existingSecrets   []runtime.Object
		existingFileRealm filerealm.Realm
		assertions        func(t *testing.T, u users)
	}{
		{
			name:              "create a new elastic user if it does not exist yet",
			existingSecrets:   nil,
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				// a random password should be generated
				require.NotEmpty(t, u[0].Password)
			},
		},
		{
			name: "elastic user secret exists but is invalid: generate a new elastic user",
			existingSecrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       nil, // no password or password removed
				},
			},
			existingFileRealm: filerealm.New().WithUser(ElasticUserName, []byte("$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6")),
			assertions: func(t *testing.T, u users) {
				// password should be regenerated
				require.NotEmpty(t, u[0].Password)
				// hash should be regenerated
				require.NotEmpty(t, u[0].PasswordHash)
				require.NotEqual(t, "$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6", u[0].PasswordHash)
			},
		},
		{
			name: "reuse the existing elastic user and password hash",
			existingSecrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       map[string][]byte{ElasticUserName: []byte("existingPassword")},
				},
			},
			existingFileRealm: filerealm.New().WithUser(ElasticUserName, []byte("$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6")),
			assertions: func(t *testing.T, u users) {
				// password and hashes should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lwsLdS0ZSyUv73WNdaRaTe8X9oeft4BoqjxtNHHH7LP7m1YImnvr6"), u[0].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if the existing one doesn't match",
			existingSecrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       map[string][]byte{ElasticUserName: []byte("existingPassword")},
				},
			},
			existingFileRealm: filerealm.New().WithUser(ElasticUserName, []byte("does-not-match-password")),
			assertions: func(t *testing.T, u users) {
				// password should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				// hash should be re-computed
				require.NotEqual(t, []byte("does-not-match-password"), u[0].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if there is none in the file realm",
			existingSecrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)},
					Data:       map[string][]byte{ElasticUserName: []byte("existingPassword")},
				},
			},
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				// password should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				// hash should be computed
				require.NotEmpty(t, u[0].PasswordHash)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.existingSecrets...)
			got, err := reconcileElasticUser(c, es, tt.existingFileRealm)
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
			err = c.Get(types.NamespacedName{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)}, &secret)
			require.NoError(t, err)
			require.Equal(t, user.Password, secret.Data[ElasticUserName])
		})
	}
}

func Test_reconcileInternalUsers(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	tests := []struct {
		name              string
		existingSecrets   []runtime.Object
		existingFileRealm filerealm.Realm
		assertions        func(t *testing.T, u users)
	}{
		{
			name:              "create new internal users if they do not exist yet",
			existingSecrets:   nil,
			existingFileRealm: filerealm.New(),
			assertions: func(t *testing.T, u users) {
				// random passwords should be generated
				require.NotEmpty(t, u[0].Password)
				require.NotEmpty(t, u[1].Password)
			},
		},
		{
			name: "reuse the existing passwords and hashes",
			existingSecrets: []runtime.Object{
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
				// passwords and hashes should be reused
				require.Equal(t, []byte("controllerUserPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG"), u[0].PasswordHash)
				require.Equal(t, []byte("probeUserPassword"), u[1].Password)
				require.Equal(t, []byte("$2a$10$8.9my2W7FVDqDnh.E1RwouN5RzkZGulQ3ZMgmoy3CH4xRvr5uYPbS"), u[1].PasswordHash)
			},
		},
		{
			name: "reuse the password but generate a new hash if the existing one doesn't match",
			existingSecrets: []runtime.Object{
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
				// password & hash of controller user should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG"), u[0].PasswordHash)
				// password of probe user should be reused, but hash should be re-computed
				require.Equal(t, []byte("probeUserPassword"), u[1].Password)
				require.NotEmpty(t, u[1].PasswordHash)
				require.NotEqual(t, "does-not-match-password", u[1].PasswordHash)

			},
		},
		{
			name: "reuse the password but generate a new hash if there is none in the file realm",
			existingSecrets: []runtime.Object{
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
				// password & hash of controller user should be reused
				require.Equal(t, []byte("existingPassword"), u[0].Password)
				require.Equal(t, []byte("$2a$10$lUuxZpa.ByS.Tid3PcMII.PrELwGjti3Mx1WRT0itwy.Ajpf.BsEG"), u[0].PasswordHash)
				// password of probe user should be reused, and hash should be re-computed
				require.Equal(t, []byte("probeUserPassword"), u[1].Password)
				require.NotEmpty(t, u[1].PasswordHash)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.existingSecrets...)
			got, err := reconcileInternalUsers(c, es, tt.existingFileRealm)
			require.NoError(t, err)
			// check returned users
			require.Len(t, got, 2)
			controllerUser := got[0]
			probeUser := got[1]
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
			err = c.Get(types.NamespacedName{Namespace: es.Namespace, Name: esv1.InternalUsersSecret(es.Name)}, &secret)
			require.NoError(t, err)
			require.Equal(t, controllerUser.Password, secret.Data[ControllerUserName])
			require.Equal(t, probeUser.Password, secret.Data[ProbeUserName])
		})
	}
}
