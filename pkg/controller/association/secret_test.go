// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var _ UnmanagedAssociation = mockUnmanagedAssociation{}

type mockUnmanagedAssociation struct {
	objSelector        commonv1.ObjectSelector
	supportsAuthAPIKey bool
}

func (m mockUnmanagedAssociation) AssociationRef() commonv1.AssociationRef {
	return m.objSelector
}

func (m mockUnmanagedAssociation) SupportsAuthAPIKey() bool {
	return m.supportsAuthAPIKey
}

func TestGetUnmanagedAssociationConnexionInfoFromSecret(t *testing.T) {
	type args struct {
		c func() k8s.Client
	}
	refObjectSelector := commonv1.ObjectSelector{Namespace: "a", Name: "b"}
	unmanagedRefSecretFixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "b"},
		Data: map[string][]byte{
			"url":      []byte("https://es.io:9243"),
			"username": []byte("elastic"),
			"password": []byte("elastic"),
		},
	}
	refObjectFixture := UnmanagedAssociationConnectionInfo{URL: "https://es.io:9243", Username: "elastic", Password: "elastic", CaCert: "", APIKey: ""}

	tests := []struct {
		name        string
		args        args
		association UnmanagedAssociation
		want        func() UnmanagedAssociationConnectionInfo
		wantErr     bool
	}{
		{
			name: "happy path with username and password",
			args: args{
				c: func() k8s.Client { return k8s.NewFakeClient(unmanagedRefSecretFixture) },
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			want:    func() UnmanagedAssociationConnectionInfo { return refObjectFixture },
			wantErr: false,
		},
		{
			name: "happy path with a ca",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					secretCopy.Data["ca.crt"] = []byte("XXXXXXXXXXXX")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			want: func() UnmanagedAssociationConnectionInfo {
				o := refObjectFixture
				o.CaCert = "XXXXXXXXXXXX"
				return o
			},
			wantErr: false,
		}, {
			name: "happy path with api-key",
			args: args{
				c: func() k8s.Client {
					return k8s.NewFakeClient(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "b"},
						Data: map[string][]byte{
							"url":     []byte("https://es.io:9243"),
							"api-key": []byte("elastic"),
						},
					})
				},
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: true,
			},
			want: func() UnmanagedAssociationConnectionInfo {
				return UnmanagedAssociationConnectionInfo{URL: "https://es.io:9243", Username: "", Password: "", CaCert: "", APIKey: "elastic"}
			},
			wantErr: false,
		}, {
			name: "secret does not exist",
			args: args{
				c: func() k8s.Client { return k8s.NewFakeClient() },
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			wantErr: true,
		},
		{
			name: "invalid secret: missing url",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					delete(secretCopy.Data, "url")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			wantErr: true,
		},
		{
			name: "invalid secret: missing username",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					delete(secretCopy.Data, "username")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			wantErr: true,
		},
		{
			name: "invalid secret: missing password",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					delete(secretCopy.Data, "password")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			wantErr: true,
		}, {
			name: "secret contains api key but association does not support it",
			args: args{
				c: func() k8s.Client {
					return k8s.NewFakeClient(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "b"},
						Data: map[string][]byte{
							"url":    []byte("https://es.io:9243"),
							"apikey": []byte("elastic"),
						},
					})
				},
			},
			association: mockUnmanagedAssociation{
				objSelector:        refObjectSelector,
				supportsAuthAPIKey: false,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUnmanagedAssociationConnectionInfoFromSecret(tt.args.c(), tt.association)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUnmanagedAssociationConnectionInfoFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil && *got != tt.want() {
				t.Errorf("GetUnmanagedAssociationConnectionInfoFromSecret() got = %v, want %v", *got, tt.want())
			}
		})
	}
}

func TestCopySecret(t *testing.T) {
	srcNSN := types.NamespacedName{Namespace: "src-ns", Name: "external-es"}
	externalESSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: srcNSN.Namespace, Name: srcNSN.Name},
		Data: map[string][]byte{
			"ca.crt":   []byte("CACERT"),
			"url":      []byte("https://es.example.com:9200"),
			"username": []byte("elastic"),
			"password": []byte("s3cr3t"),
		},
	}

	hashOf := func(t *testing.T, secret *corev1.Secret, keys []string) uint32 {
		t.Helper()
		h := fnv.New32a()
		_, err := copySecret(context.Background(), k8s.NewFakeClient(secret), h, "dst-ns", srcNSN, keys, srcNSN.Name, nil, nil)
		require.NoError(t, err)
		return h.Sum32()
	}

	hashNoFilter := hashOf(t, externalESSecret, nil)
	hashCAFilter := hashOf(t, externalESSecret, []string{"ca.crt"})

	rotated := externalESSecret.DeepCopy()
	rotated.Data["password"] = []byte("new-password")

	secretWithLastApplied := externalESSecret.DeepCopy()
	secretWithLastApplied.Annotations = map[string]string{
		corev1.LastAppliedConfigAnnotation: `{"data":{"password":"czNjcjN0"}}`,
		"other-annotation":                 "keep-me",
	}

	for _, tt := range []struct {
		name                string
		srcSecret           *corev1.Secret
		existingTarget      *corev1.Secret
		targetNamespace     string // defaults to "dst-ns" when empty
		targetName          string // defaults to srcNSN.Name when empty
		keys                []string
		owner               client.Object
		extraLabels         map[string]string
		wantCopied          bool
		wantKeys            []string
		wantMissKeys        []string
		wantMissAnnotations []string
		wantAnnotations     map[string]string
		wantLabels          map[string]string
		wantOwnerName       string
		wantHash            *uint32
		wantHashNot         *uint32
	}{
		{
			name:       "no filter: all keys copied",
			srcSecret:  externalESSecret,
			keys:       nil,
			wantCopied: true,
			wantKeys:   []string{"ca.crt", "url", "username", "password"},
		},
		{
			name:                "no filter: last-applied-configuration stripped",
			srcSecret:           secretWithLastApplied,
			keys:                nil,
			wantCopied:          true,
			wantKeys:            []string{"ca.crt", "url", "username", "password"},
			wantMissAnnotations: []string{corev1.LastAppliedConfigAnnotation},
			wantAnnotations:     map[string]string{"other-annotation": "keep-me"},
		},
		{
			name:         "ca.crt filter: only ca.crt copied, credentials absent",
			srcSecret:    externalESSecret,
			keys:         []string{"ca.crt"},
			wantCopied:   true,
			wantKeys:     []string{"ca.crt"},
			wantMissKeys: []string{"url", "username", "password"},
			wantHashNot:  &hashNoFilter,
		},
		{
			// Password rotation must not change the hash when only ca.crt is filtered in,
			// so that credential rotations don't trigger unnecessary fleet-agent pod restarts.
			name:         "password rotation does not change hash when ca.crt filter is active",
			srcSecret:    rotated,
			keys:         []string{"ca.crt"},
			wantCopied:   true,
			wantKeys:     []string{"ca.crt"},
			wantMissKeys: []string{"url", "username", "password"},
			wantHash:     &hashCAFilter,
		},
		{
			name:         "absent key in filter is skipped with a debug log",
			srcSecret:    externalESSecret,
			keys:         []string{"ca.crt", "nonexistent-key"},
			wantCopied:   true,
			wantKeys:     []string{"ca.crt"},
			wantMissKeys: []string{"nonexistent-key", "url", "username", "password"},
		},
		{
			// kubectl.kubernetes.io/last-applied-configuration embeds the full original
			// Secret manifest as base64; it must not appear in the cross-namespace copy.
			name:                "last-applied-configuration annotation stripped, other annotations kept",
			srcSecret:           secretWithLastApplied,
			keys:                []string{"ca.crt"},
			wantCopied:          true,
			wantKeys:            []string{"ca.crt"},
			wantMissAnnotations: []string{corev1.LastAppliedConfigAnnotation},
			wantAnnotations:     map[string]string{"other-annotation": "keep-me"},
		},
		{
			name:      "last-applied-configuration removed from pre-existing copy on upgrade",
			srcSecret: externalESSecret,
			existingTarget: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "dst-ns",
					Name:      srcNSN.Name,
					Annotations: map[string]string{
						corev1.LastAppliedConfigAnnotation: `{"data":{"password":"czNjcjN0","username":"ZWxhc3RpYw=="}}`,
					},
				},
				Data: map[string][]byte{"ca.crt": []byte("CACERT")},
			},
			keys:                []string{"ca.crt"},
			wantCopied:          true,
			wantKeys:            []string{"ca.crt"},
			wantMissAnnotations: []string{corev1.LastAppliedConfigAnnotation},
		},
		{
			name:          "owner reference is set on copy",
			srcSecret:     externalESSecret,
			keys:          []string{"ca.crt"},
			owner:         &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "dst-ns", UID: "test-uid"}},
			wantCopied:    true,
			wantKeys:      []string{"ca.crt"},
			wantOwnerName: "agent1",
		},
		{
			name: "extra labels are merged onto source labels in copy",
			srcSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: srcNSN.Namespace, Name: srcNSN.Name,
					Labels: map[string]string{"src-label": "src-value"},
				},
				Data: map[string][]byte{"ca.crt": []byte("CACERT")},
			},
			extraLabels: map[string]string{"extra-label": "extra-value"},
			wantCopied:  true,
			wantKeys:    []string{"ca.crt"},
			wantLabels:  map[string]string{"src-label": "src-value", "extra-label": "extra-value"},
		},
		{
			name:            "same-namespace: hash written, no copy created",
			srcSecret:       externalESSecret,
			keys:            []string{"ca.crt"},
			targetNamespace: srcNSN.Namespace,
			wantCopied:      false,
		},
		{
			name:       "custom targetName: copy created under targetName, not source name",
			srcSecret:  externalESSecret,
			keys:       []string{"ca.crt"},
			targetName: "agent1-agent-fleetserver-hashxxx-es-ca",
			wantCopied: true,
			wantKeys:   []string{"ca.crt"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var fakeClient k8s.Client
			if tt.existingTarget != nil {
				fakeClient = k8s.NewFakeClient(tt.srcSecret, tt.existingTarget)
			} else {
				fakeClient = k8s.NewFakeClient(tt.srcSecret)
			}
			targetNS := tt.targetNamespace
			if targetNS == "" {
				targetNS = "dst-ns"
			}
			tgtName := tt.targetName
			if tgtName == "" {
				tgtName = srcNSN.Name
			}
			h := fnv.New32a()
			wasCopied, err := copySecret(context.Background(), fakeClient, h, targetNS, srcNSN, tt.keys, tgtName, tt.owner, tt.extraLabels)
			require.NoError(t, err)
			require.Equal(t, tt.wantCopied, wasCopied)
			if !tt.wantCopied {
				return
			}

			var copied corev1.Secret
			require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: tgtName}, &copied))
			// copySecret must always mark cross-namespace copies so deleteStaleAdditionalSecrets can GC them.
			assert.Equal(t, "true", copied.Labels[AdditionalSecretLabelName], "cross-namespace copy must carry AdditionalSecretLabelName label")
			if tt.targetName != "" {
				// Confirm the copy does NOT exist under the source name when targetName overrides it.
				var ghost corev1.Secret
				err := fakeClient.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: srcNSN.Name}, &ghost)
				assert.True(t, apierrors.IsNotFound(err), "copy must not exist under source name %q when targetName %q overrides it", srcNSN.Name, tt.targetName)
			}

			for _, key := range tt.wantKeys {
				assert.Contains(t, copied.Data, key, "expected key %q in copied secret", key)
			}
			for _, key := range tt.wantMissKeys {
				assert.NotContains(t, copied.Data, key, "credential key %q must not appear in copied secret", key)
			}
			for _, ann := range tt.wantMissAnnotations {
				assert.NotContains(t, copied.Annotations, ann, "annotation %q must not appear in copied secret", ann)
			}
			for k, v := range tt.wantAnnotations {
				assert.Equal(t, v, copied.Annotations[k], "annotation %q must be preserved in copied secret", k)
			}
			for k, v := range tt.wantLabels {
				assert.Equal(t, v, copied.Labels[k], "label %q must be present in copied secret", k)
			}
			if tt.wantOwnerName != "" {
				require.Len(t, copied.OwnerReferences, 1)
				assert.Equal(t, tt.wantOwnerName, copied.OwnerReferences[0].Name)
			}
			if tt.wantHash != nil {
				assert.Equal(t, *tt.wantHash, h.Sum32())
			}
			if tt.wantHashNot != nil {
				assert.NotEqual(t, *tt.wantHashNot, h.Sum32())
			}
		})
	}
}
