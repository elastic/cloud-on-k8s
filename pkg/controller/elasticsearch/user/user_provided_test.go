// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func initDynamicWatches(watchNames ...string) watches.DynamicWatches {
	controllerscheme.SetupScheme()
	w := watches.NewDynamicWatches()
	for _, name := range watchNames {
		_ = w.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{
			Name: name,
		})
	}
	return w
}

var sampleEsWithAuth = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
	Spec: esv1.ElasticsearchSpec{
		Auth: esv1.Auth{
			FileRealm: []esv1.FileRealmSource{
				{SecretRef: v1.SecretRef{SecretName: "filerealm-secret-1"}},
				{SecretRef: v1.SecretRef{SecretName: "filerealm-secret-2"}},
			},
			Roles: []esv1.RoleSource{
				{SecretRef: v1.SecretRef{SecretName: "roles-secret-1"}},
				{SecretRef: v1.SecretRef{SecretName: "roles-secret-2"}},
			},
		},
		Version: "8.10.0",
	},
}
var sampleUserProvidedFileRealmSecrets = []client.Object{
	&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "filerealm-secret-1"},
		Data: map[string][]byte{
			filerealm.UsersFile:      []byte("user1:hash1\nuser2:hash2"),
			filerealm.UsersRolesFile: []byte("role1:user1,user2\nrole2:user1\nrole3:"),
		},
	},
	&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "filerealm-secret-2"},
		Data: map[string][]byte{
			// different from 1st secret, should have priority
			filerealm.UsersFile: []byte("user1:otherhash1\nuser3:hash3"),
			// should be merged with role mapping from 1st secret
			filerealm.UsersRolesFile: []byte("role1:user1,user3\nrole4:user1,user2"),
		},
	},
}

var sampleUserProvidedRolesSecret = []client.Object{
	&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "roles-secret-1"},
		Data: map[string][]byte{
			RolesFile: []byte("role1: rolespec1\nrole2: rolespec2"),
		},
	},
	&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "roles-secret-2"},
		Data: map[string][]byte{
			RolesFile: []byte("role1: rolespec1updated\nrole2: rolespec2"), // different from the 1st secret, should have priority
		},
	},
}

func TestReconcileUserProvidedFileRealm(t *testing.T) {
	tests := []struct {
		name          string
		es            esv1.Elasticsearch
		secrets       []client.Object
		existingRealm filerealm.Realm
		watched       watches.DynamicWatches
		wantWatched   []string
		wantFileRealm filerealm.Realm
		wantEvents    int
	}{
		{
			name:          "no auth provided",
			es:            esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}},
			secrets:       nil,
			watched:       initDynamicWatches(),
			wantWatched:   []string{},
			wantFileRealm: filerealm.New(),
		},
		{
			name:        "aggregate users from multiple secrets",
			es:          sampleEsWithAuth,
			secrets:     sampleUserProvidedFileRealmSecrets,
			watched:     initDynamicWatches(),
			wantWatched: []string{UserProvidedFileRealmWatchName(types.NamespacedName{Namespace: "ns", Name: "es"})},
			wantFileRealm: filerealm.New().
				WithUser("user1", []byte("otherhash1")).
				WithUser("user2", []byte("hash2")).
				WithUser("user3", []byte("hash3")).
				WithRole("role1", []string{"user1", "user2", "user3"}).
				WithRole("role2", []string{"user1"}).
				WithRole("role3", nil).
				WithRole("role4", []string{"user1", "user2"}),
		},
		{
			name: "unknown secret referenced: emit an event but don't error out",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{Auth: esv1.Auth{FileRealm: []esv1.FileRealmSource{
					{SecretRef: v1.SecretRef{SecretName: "unknown-secret"}},
					{SecretRef: v1.SecretRef{SecretName: "unknown-secret-2"}},
				}}},
			},
			secrets:       nil,
			watched:       initDynamicWatches(),
			wantWatched:   []string{UserProvidedFileRealmWatchName(types.NamespacedName{Namespace: "ns", Name: "es"})},
			wantFileRealm: filerealm.New(),
			wantEvents:    2,
		},
		{
			name: "invalid secret data: emit an event but don't error out",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{Auth: esv1.Auth{FileRealm: []esv1.FileRealmSource{
					{SecretRef: v1.SecretRef{SecretName: "invalid-secret"}},
				}}},
			},
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "invalid-secret"},
					Data: map[string][]byte{
						filerealm.UsersFile: []byte("invalid-data"),
					},
				},
			},
			watched:       initDynamicWatches(),
			wantWatched:   []string{UserProvidedFileRealmWatchName(types.NamespacedName{Namespace: "ns", Name: "es"})},
			wantFileRealm: filerealm.New(),
			wantEvents:    1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := record.NewFakeRecorder(10)
			c := k8s.NewFakeClient(tt.secrets...)
			gotFileRealm, err := reconcileUserProvidedFileRealm(context.Background(), c, tt.es, filerealm.New(), tt.watched, recorder, testPasswordHasher)
			require.NoError(t, err)
			require.Equal(t, tt.wantFileRealm, gotFileRealm)
			require.Equal(t, tt.wantWatched, tt.watched.Secrets.Registrations())
			require.Len(t, recorder.Events, tt.wantEvents)
		})
	}
}

func TestReconcileUserProvidedRoles(t *testing.T) {
	tests := []struct {
		name        string
		es          esv1.Elasticsearch
		secrets     []client.Object
		watched     watches.DynamicWatches
		wantWatched []string
		wantRoles   RolesFileContent
		wantEvents  int
	}{
		{
			name:        "no auth provided",
			es:          esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}},
			secrets:     nil,
			watched:     initDynamicWatches(),
			wantWatched: []string{},
			wantRoles:   RolesFileContent{},
		},
		{
			name:        "aggregate users and roles from multiple secrets",
			es:          sampleEsWithAuth,
			secrets:     sampleUserProvidedRolesSecret,
			watched:     initDynamicWatches(),
			wantWatched: []string{UserProvidedRolesWatchName(types.NamespacedName{Namespace: "ns", Name: "es"})},
			wantRoles: RolesFileContent{
				"role1": "rolespec1updated",
				"role2": "rolespec2",
			},
		},
		{
			name: "unknown secret referenced: emit an event but don't error out",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{Auth: esv1.Auth{Roles: []esv1.RoleSource{
					{SecretRef: v1.SecretRef{SecretName: "unknown-secret"}},
					{SecretRef: v1.SecretRef{SecretName: "unknown-secret-2"}},
				}}},
			},
			secrets:     nil,
			watched:     initDynamicWatches(),
			wantWatched: []string{UserProvidedRolesWatchName(types.NamespacedName{Namespace: "ns", Name: "es"})},
			wantRoles:   RolesFileContent{},
			wantEvents:  2,
		},
		{
			name: "invalid secret data: emit an event but don't error out",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{Auth: esv1.Auth{Roles: []esv1.RoleSource{
					{SecretRef: v1.SecretRef{SecretName: "invalid-secret"}},
				}}},
			},
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "invalid-secret"},
					Data: map[string][]byte{
						RolesFile: []byte("[invalid yaml]"),
					},
				},
			},
			watched:     initDynamicWatches(),
			wantWatched: []string{UserProvidedRolesWatchName(types.NamespacedName{Namespace: "ns", Name: "es"})},
			wantRoles:   RolesFileContent{},
			wantEvents:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := record.NewFakeRecorder(10)
			c := k8s.NewFakeClient(tt.secrets...)
			gotRoles, err := reconcileUserProvidedRoles(context.Background(), c, tt.es, tt.watched, recorder)
			require.NoError(t, err)
			require.Equal(t, tt.wantRoles, gotRoles)
			require.Equal(t, tt.wantWatched, tt.watched.Secrets.Registrations())
			require.Len(t, recorder.Events, tt.wantEvents)
		})
	}
}

func Test_realmFromBasicAuthSecret(t *testing.T) {
	realmPtr := func(r filerealm.Realm) *filerealm.Realm {
		return &r
	}
	type args struct {
		secret   corev1.Secret
		existing filerealm.Realm
	}
	testUser := "my-user"
	basicAuthSecretFixture := corev1.Secret{
		Data: map[string][]byte{
			"username": []byte(testUser),
			"password": []byte("my-user-pass"),
		},
	}
	tests := []struct {
		name         string
		args         args
		wantEqual    *filerealm.Realm
		wantPassword string
		wantErr      bool
	}{
		{
			name: "missing username",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						"password": nil,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid user",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						"username": []byte(testUser),
						"password": []byte(""),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing password",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						"username": []byte(testUser),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "users file specified",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						"users": nil,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "reuses existing hash",
			args: args{
				secret: basicAuthSecretFixture,
				existing: filerealm.New().
					WithUser(testUser, []byte("$2a$10$aQlJpc7r/5SMPaXJil8tyOUr3pPOrhyyPVRMIDdUDkbGS.T0kU776")),
			},
			wantEqual: realmPtr(filerealm.New().
				WithUser(testUser, []byte("$2a$10$aQlJpc7r/5SMPaXJil8tyOUr3pPOrhyyPVRMIDdUDkbGS.T0kU776"))),
			wantErr: false,
		},
		{
			name: "supports user role definition",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						"username": []byte(testUser),
						"password": []byte("my-user-pass"),
						"roles":    []byte("superuser"),
					},
				},
				existing: filerealm.New().
					WithUser(testUser, []byte("$2a$10$aQlJpc7r/5SMPaXJil8tyOUr3pPOrhyyPVRMIDdUDkbGS.T0kU776")),
			},
			wantEqual: realmPtr(filerealm.New().
				WithRole("superuser", []string{testUser}).
				WithUser(testUser, []byte("$2a$10$aQlJpc7r/5SMPaXJil8tyOUr3pPOrhyyPVRMIDdUDkbGS.T0kU776"))),
			wantErr: false,
		},
		{
			name: "creates new password hash in absence of existing file realm",
			args: args{
				secret: basicAuthSecretFixture,
			},
			wantPassword: "my-user-pass",
			wantErr:      false,
		},
		{
			name: "Generate new hash without error if current one is invalid",
			args: args{
				secret: basicAuthSecretFixture,
				existing: filerealm.New().
					WithUser(testUser, []byte("$2a$10$invalidhash.invalidhash")),
			},
			wantPassword: "my-user-pass",
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := realmFromBasicAuthSecret(tt.args.secret, tt.args.existing, testPasswordHasher)
			if (err != nil) != tt.wantErr {
				t.Errorf("realmFromBasicAuthSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return // no need to compare resulting realm in presence of errors
			}

			if tt.wantEqual != nil && !reflect.DeepEqual(got, *tt.wantEqual) {
				t.Errorf("realmFromBasicAuthSecret() got = %v, want %v", got, tt.wantEqual)
			}
			if tt.wantEqual == nil && bcrypt.CompareHashAndPassword(got.PasswordHashForUser(testUser), []byte(tt.wantPassword)) != nil {
				t.Errorf("realmFromBasicAuthSecret() got = %v, does not match %v", got, tt.wantPassword)
			}
		})
	}
}
