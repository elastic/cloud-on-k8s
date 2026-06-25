// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password/fixtures"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user/filerealm"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var testPasswordHasher cryptutil.PasswordHasher

func init() {
	passwordHasher, err := cryptutil.NewPasswordHasher(0)
	if err != nil {
		panic(err)
	}
	testPasswordHasher = passwordHasher
}

func TestReconcileUsersAndRoles(t *testing.T) {
	c := k8s.NewFakeClient(append(sampleUserProvidedFileRealmSecrets, sampleUserProvidedRolesSecret...)...)
	controllerUser, err := ReconcileUsersAndRoles(t.Context(), c, sampleEsWithAuth, initDynamicWatches(), toolsevents.NewFakeRecorder(10), testPasswordHasher, fixtures.MustTestRandomGenerator(16), PolicyRoles{}, metadata.Metadata{})
	require.NoError(t, err)
	require.NotEmpty(t, controllerUser.Password)
	var reconciledSecret corev1.Secret
	err = c.Get(t.Context(), RolesFileRealmSecretKey(sampleEsWithAuth), &reconciledSecret)
	require.NoError(t, err)
	require.Len(t, reconciledSecret.Data, 4)
	require.Equal(t, commonv1.RestrictWatchedResourcesLabelValue, reconciledSecret.Labels[commonv1.RestrictWatchedResourcesLabelName])
	require.NotEmpty(t, reconciledSecret.Data[RolesFile])
	require.NotEmpty(t, reconciledSecret.Data[filerealm.UsersRolesFile])
	require.NotEmpty(t, reconciledSecret.Data[filerealm.UsersFile])
}

func Test_reconcileRolesFileRealmSecret(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}

	clickAdminsRoles := RolesFileContent{"click_admins": []byte(`run_as: [ 'clicks_watcher_1' ]
  cluster: [ 'monitor' ]
  indices:
  - names: [ 'events-*' ]
    privileges: [ 'read' ]
    field_security:
      grant: ['category', '@timestamp', 'message' ]
    query: '{"match": {"category": "click"}}'`)}

	policyRoles := RolesFileContent{"scp_role": map[string]any{
		"cluster": []any{"monitor"},
		"indices": []any{map[string]any{
			"names":      []any{"logs-*"},
			"privileges": []any{"read"},
		}},
	}}

	fullRealm := filerealm.New().
		WithUser("user1", []byte("hash1")).
		WithUser("user2", []byte("hash2")).
		WithRole("role1", []string{"user1"}).
		WithRole("role2", []string{"user2"})

	fullSATokens := ServiceAccountTokens{}.
		Add(ServiceAccountToken{FullyQualifiedServiceAccountName: "fqsa2", HashedSecret: "hash2"}).
		Add(ServiceAccountToken{FullyQualifiedServiceAccountName: "fqsa1", HashedSecret: "hash1"})

	tests := []struct {
		name               string
		roles              RolesFileContent
		realm              filerealm.Realm
		saTokens           ServiceAccountTokens
		inputMeta          metadata.Metadata
		policyRolesHash    string
		wantDataLen        int
		wantDataContains   map[string]string
		wantAnnotationHash string
		setup              func(t *testing.T, c k8s.Client)
		check              func(t *testing.T, inputMeta metadata.Metadata)
	}{
		{
			name:        "no policyRolesHash - full data reconciliation",
			roles:       clickAdminsRoles,
			realm:       fullRealm,
			saTokens:    fullSATokens,
			wantDataLen: 4,
			wantDataContains: map[string]string{
				RolesFile:                "click_admins",
				filerealm.UsersRolesFile: "role1:user1",
				filerealm.UsersFile:      "user1:hash1",
				ServiceTokensFileName:    "fqsa1:hash1\nfqsa2:hash2\n",
			},
			wantAnnotationHash: "",
		},
		{
			name:               "with policyRolesHash - annotation set to hash",
			roles:              policyRoles,
			realm:              filerealm.New(),
			policyRolesHash:    "abc123hash",
			wantDataContains:   map[string]string{RolesFile: "scp_role"},
			wantAnnotationHash: "abc123hash",
		},
		{
			name:               "metadata.Annotations is not mutated by roles hash injection",
			roles:              policyRoles,
			realm:              filerealm.New(),
			policyRolesHash:    "abc123hash",
			inputMeta:          metadata.Metadata{Annotations: map[string]string{"existing": "value"}},
			wantAnnotationHash: "abc123hash",
			check: func(t *testing.T, inputMeta metadata.Metadata) {
				t.Helper()
				_, hasRolesHash := inputMeta.Annotations[commonannotation.ElasticsearchRolesHashAnnotation]
				require.False(t, hasRolesHash, "inputMeta.Annotations must not be mutated")
			},
		},
		{
			name:               "metadata.Labels is not mutated by RestrictWatchedResources label injection",
			roles:              policyRoles,
			realm:              filerealm.New(),
			policyRolesHash:    "abc123hash",
			inputMeta:          metadata.Metadata{Labels: map[string]string{"existing": "value"}},
			wantAnnotationHash: "abc123hash",
			check: func(t *testing.T, inputMeta metadata.Metadata) {
				t.Helper()
				_, hasRestrict := inputMeta.Labels[commonv1.RestrictWatchedResourcesLabelName]
				require.False(t, hasRestrict, "inputMeta.Labels must not be mutated")
			},
		},
		{
			name:               "clearing policyRolesHash empties the annotation",
			roles:              policyRoles,
			realm:              filerealm.New(),
			policyRolesHash:    "",
			wantAnnotationHash: "",
			setup: func(t *testing.T, c k8s.Client) {
				t.Helper()
				err := reconcileRolesFileRealmSecret(t.Context(), c, es, policyRoles, filerealm.New(), ServiceAccountTokens{}, "pre-existing-hash", metadata.Metadata{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient()
			if tt.setup != nil {
				tt.setup(t, c)
			}

			inputMeta := tt.inputMeta
			err := reconcileRolesFileRealmSecret(t.Context(), c, es, tt.roles, tt.realm, tt.saTokens, tt.policyRolesHash, inputMeta)
			require.NoError(t, err)

			var secret corev1.Secret
			require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.RolesAndFileRealmSecret(es.Name)}, &secret))

			if tt.wantDataLen > 0 {
				require.Len(t, secret.Data, tt.wantDataLen)
			}
			for file, substr := range tt.wantDataContains {
				require.Contains(t, string(secret.Data[file]), substr, "file %q", file)
			}
			require.Equal(t, commonv1.RestrictWatchedResourcesLabelValue, secret.Labels[commonv1.RestrictWatchedResourcesLabelName])
			rolesHash, ok := secret.Annotations[commonannotation.ElasticsearchRolesHashAnnotation]
			require.True(t, ok, "ElasticsearchRolesHashAnnotation must always be present")
			require.Equal(t, tt.wantAnnotationHash, rolesHash)

			if tt.check != nil {
				tt.check(t, inputMeta)
			}
		})
	}
}

func Test_aggregateFileRealm(t *testing.T) {
	sampleEsWithAuthAndElasticUserDisabled := sampleEsWithAuth.DeepCopy()
	sampleEsWithAuthAndElasticUserDisabled.Spec.Auth.DisableElasticUser = true
	tests := []struct {
		name       string
		es         esv1.Elasticsearch
		expected   []string
		assertions func(t *testing.T, c k8s.Client, es esv1.Elasticsearch)
	}{
		{
			name:     "file realm users with elastic user enabled",
			es:       sampleEsWithAuth,
			expected: []string{"elastic", "elastic-internal", "elastic-internal-pre-stop", "elastic-internal-probe", "elastic-internal-diagnostics", "elastic-internal-monitoring", "user1", "user2", "user3"},
		},
		{
			name:     "file realm users with elastic user disabled",
			es:       *sampleEsWithAuthAndElasticUserDisabled,
			expected: []string{"elastic-internal", "elastic-internal-pre-stop", "elastic-internal-probe", "elastic-internal-diagnostics", "elastic-internal-monitoring", "user1", "user2", "user3"},
			assertions: func(t *testing.T, c k8s.Client, es esv1.Elasticsearch) {
				t.Helper()
				var secret corev1.Secret
				err := c.Get(t.Context(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.ElasticUserSecret(es.Name)}, &secret)
				require.True(t, apierrors.IsNotFound(err))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(sampleUserProvidedFileRealmSecrets...)
			fileRealm, controllerUser, err := aggregateFileRealm(t.Context(), c, tt.es, initDynamicWatches(), toolsevents.NewFakeRecorder(10), testPasswordHasher, fixtures.MustTestRandomGenerator(16), metadata.Metadata{})
			require.NoError(t, err)
			require.NotEmpty(t, controllerUser.Password)
			actualUsers := fileRealm.UserNames()
			require.ElementsMatch(t, tt.expected, actualUsers)
			if tt.assertions != nil {
				tt.assertions(t, c, tt.es)
			}
		})
	}
}

func Test_aggregateRoles(t *testing.T) {
	esWithOverlap := sampleEsWithAuth.DeepCopy()
	esWithOverlap.Spec.Auth.Roles = []esv1.RoleSource{{SecretRef: commonv1.SecretRef{SecretName: "user-roles"}}}

	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		makeClient   func() k8s.Client
		policyRoles  RolesFileContent
		wantLen      int
		wantContains []string
		check        func(t *testing.T, roles RolesFileContent)
	}{
		{
			name:         "nil SCP roles - predefined and user-provided only",
			es:           sampleEsWithAuth,
			makeClient:   func() k8s.Client { return k8s.NewFakeClient(sampleUserProvidedRolesSecret...) },
			policyRoles:  nil,
			wantLen:      57,
			wantContains: []string{ProbeUserRole, ClusterManageRole, "role1", "role2"},
		},
		{
			name:         "empty SCP roles is equivalent to nil",
			es:           sampleEsWithAuth,
			makeClient:   func() k8s.Client { return k8s.NewFakeClient(sampleUserProvidedRolesSecret...) },
			policyRoles:  map[string]any{},
			wantLen:      57,
			wantContains: []string{ProbeUserRole, "role1", "role2"},
		},
		{
			name:       "with SCP roles - SCP role is added",
			es:         sampleEsWithAuth,
			makeClient: func() k8s.Client { return k8s.NewFakeClient(sampleUserProvidedRolesSecret...) },
			policyRoles: map[string]any{
				"scp_monitoring_role": map[string]any{
					"cluster": []any{"monitor"},
					"indices": []any{map[string]any{
						"names":      []any{".monitoring-*"},
						"privileges": []any{"read"},
					}},
				},
			},
			// 55 predefined + 2 user-provided + 1 SCP role
			wantLen:      58,
			wantContains: []string{"scp_monitoring_role", "role1", "role2"},
		},
		{
			name: "SCP role overrides user-provided role with the same name",
			es:   *esWithOverlap,
			makeClient: func() k8s.Client {
				return k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: sampleEsWithAuth.Namespace, Name: "user-roles"},
					Data:       map[string][]byte{RolesFile: []byte("shared_role:\n  cluster:\n    - monitor\n")},
				})
			},
			policyRoles:  map[string]any{"shared_role": map[string]any{"cluster": []any{"manage"}}},
			wantContains: []string{"shared_role"},
			check: func(t *testing.T, roles RolesFileContent) {
				t.Helper()
				roleSpec, ok := roles["shared_role"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, []any{"manage"}, roleSpec["cluster"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.makeClient()
			roles, err := aggregateRoles(t.Context(), c, tt.es, initDynamicWatches(), toolsevents.NewFakeRecorder(10), tt.policyRoles)
			require.NoError(t, err)
			if tt.wantLen > 0 {
				require.Len(t, roles, tt.wantLen)
			}
			for _, role := range tt.wantContains {
				require.Contains(t, roles, role)
			}
			if tt.check != nil {
				tt.check(t, roles)
			}
		})
	}
}
