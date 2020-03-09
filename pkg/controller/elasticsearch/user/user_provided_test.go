// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	commonscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func initDynamicWatches(watchNames ...string) watches.DynamicWatches {
	w := watches.NewDynamicWatches()
	_ = commonscheme.SetupScheme()
	_ = w.Secrets.InjectScheme(scheme.Scheme)
	for _, name := range watchNames {
		_ = w.Secrets.AddHandler(watches.NamedWatch{
			Name: name,
		})
	}
	return w
}

var sampleEsWithAuth = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
	Spec: esv1.ElasticsearchSpec{Auth: esv1.Auth{
		FileRealm: []esv1.FileRealmSource{
			{SecretRef: v1.SecretRef{SecretName: "filerealm-secret-1"}},
			{SecretRef: v1.SecretRef{SecretName: "filerealm-secret-2"}},
		},
		Roles: []esv1.RoleSource{
			{SecretRef: v1.SecretRef{SecretName: "roles-secret-1"}},
			{SecretRef: v1.SecretRef{SecretName: "roles-secret-2"}},
		},
	}},
}
var sampleUserProvidedFileRealmSecrets = []runtime.Object{
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

var sampleUserProvidedRolesSecret = []runtime.Object{
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
		secrets       []runtime.Object
		watched       watches.DynamicWatches
		wantWatched   []string
		wantFileRealm filerealm.Realm
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.secrets...)
			gotFileRealm, err := reconcileUserProvidedFileRealm(c, tt.es, tt.watched)
			require.NoError(t, err)
			require.Equal(t, tt.wantFileRealm, gotFileRealm)
			require.Equal(t, tt.wantWatched, tt.watched.Secrets.Registrations())
		})
	}
}

func TestReconcileUserProvidedRoles(t *testing.T) {
	tests := []struct {
		name        string
		es          esv1.Elasticsearch
		secrets     []runtime.Object
		watched     watches.DynamicWatches
		wantWatched []string
		wantRoles   RolesFileContent
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.secrets...)
			gotRoles, err := reconcileUserProvidedRoles(c, tt.es, tt.watched)
			require.NoError(t, err)
			require.Equal(t, tt.wantRoles, gotRoles)
			require.Equal(t, tt.wantWatched, tt.watched.Secrets.Registrations())
		})
	}
}
