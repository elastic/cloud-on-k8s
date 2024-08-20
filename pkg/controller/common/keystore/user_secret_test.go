// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var kbNamer = name.NewNamer("kb")

func Test_secureSettingsWatchName(t *testing.T) {
	require.Equal(t, "ns-name-secure-settings", SecureSettingsWatchName(types.NamespacedName{Namespace: "ns", Name: "name"}))
}

func Test_secureSettingsVolume(t *testing.T) {
	expectedSecretVolume := volume.NewSecretVolumeWithMountPath(
		"kibana-kb-secure-settings",
		SecureSettingsVolumeName,
		SecureSettingsVolumeMountPath,
	)
	createWatches := func(handlerName string) watches.DynamicWatches {
		w := watches.NewDynamicWatches()
		if handlerName != "" {
			require.NoError(t, w.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{
				Name: handlerName,
			}))
		}
		return w
	}
	tests := []struct {
		name        string
		c           k8s.Client
		w           watches.DynamicWatches
		kb          kbv1.Kibana
		wantVolume  *volume.SecretVolume
		wantHash    string
		wantWatches []string
		wantEvent   string
	}{
		{
			name:        "no secure settings specified in Kibana spec: should return nothing",
			c:           k8s.NewFakeClient(),
			w:           createWatches(""),
			kb:          testKibana,
			wantVolume:  nil,
			wantHash:    "",
			wantWatches: []string{},
		},
		{
			name:       "valid secure settings specified: should add watch and return volume with version",
			c:          k8s.NewFakeClient(&testSecureSettingsSecret),
			w:          createWatches(""),
			kb:         testKibanaWithSecureSettings,
			wantVolume: &expectedSecretVolume,
			// since this is being created the RV will increment
			wantHash:    "896069204",
			wantWatches: []string{SecureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))},
		},
		{
			name:        "secure setting specified but no secret exists: should return nothing but watch the secret, and emit an event",
			c:           k8s.NewFakeClient(),
			w:           createWatches(SecureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))),
			kb:          testKibanaWithSecureSettings,
			wantVolume:  nil,
			wantHash:    "",
			wantWatches: []string{SecureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))},
			wantEvent:   "Warning Unexpected Secure settings secret not found: namespace/secure-settings-secret",
		},
		{
			name:        "secure settings removed (was set before): should remove watch",
			c:           k8s.NewFakeClient(&testSecureSettingsSecret),
			w:           createWatches(SecureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))),
			kb:          testKibana,
			wantVolume:  nil,
			wantHash:    "",
			wantWatches: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDriver := driver.TestDriver{
				Client:       tt.c,
				Watches:      tt.w,
				FakeRecorder: record.NewFakeRecorder(1000),
			}
			vol, hash, err := secureSettingsVolume(context.Background(), testDriver, &tt.kb, nil, kbNamer)
			require.NoError(t, err)
			assert.Equal(t, tt.wantVolume, vol)
			assert.Equal(t, tt.wantHash, hash)

			require.Equal(t, tt.wantWatches, tt.w.Secrets.Registrations())

			if tt.wantEvent != "" {
				require.Equal(t, tt.wantEvent, <-testDriver.FakeRecorder.Events)
			} else {
				// no event expected
				select {
				case e := <-testDriver.FakeRecorder.Events:
					require.Fail(t, "no event expected but got one", "event", e)
				default:
					// ok
				}
			}
		})
	}
}

func Test_reconcileSecureSettings(t *testing.T) {
	trueVal := true
	type args struct {
		c           k8s.Client
		hasKeystore HasKeystore
		userSecrets []corev1.Secret
		namer       name.Namer
	}
	kibanaFixture := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb",
			Namespace: "ns",
		},
	}
	expectedMeta := metav1.ObjectMeta{
		Name:      "kb-kb-secure-settings",
		Namespace: "ns",
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion:         "kibana.k8s.elastic.co/v1",
				Kind:               "Kibana",
				Name:               "kb",
				UID:                "",
				Controller:         &trueVal,
				BlockOwnerDeletion: &trueVal,
			},
		},
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.Secret
		wantErr bool
	}{
		{
			name: "empty user secret",
			args: args{
				c:           k8s.NewFakeClient(),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{},
				},
				namer: kbNamer,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "new user secret",
			args: args{
				c:           k8s.NewFakeClient(),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{
						Data: map[string][]byte{
							"key1": []byte("value1"),
						}},
				},
				namer: kbNamer,
			},
			want: &corev1.Secret{
				ObjectMeta: expectedMeta,
				Data: map[string][]byte{
					"key1": []byte("value1"),
				}},
			wantErr: false,
		},
		{
			name: "updated existing secret",
			args: args{
				c: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: expectedMeta,
					Data: map[string][]byte{
						"key1": []byte("old-value"),
					},
				}),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{
						Data: map[string][]byte{
							"key1": []byte("value1"),
						},
					},
				},
				namer: kbNamer,
			},
			want: &corev1.Secret{
				ObjectMeta: expectedMeta,
				Data: map[string][]byte{
					"key1": []byte("value1"),
				}},
			wantErr: false,
		},
		{
			name: "secure settings removed",
			args: args{
				c: k8s.NewFakeClient(&corev1.Secret{
					ObjectMeta: expectedMeta,
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				}),
				hasKeystore: kibanaFixture,
				userSecrets: nil,
				namer:       kbNamer,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "no secure settings and no previous settings",
			args: args{
				c:           k8s.NewFakeClient(),
				hasKeystore: kibanaFixture,
				userSecrets: nil,
				namer:       kbNamer,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "multiple user secrets",
			args: args{
				c:           k8s.NewFakeClient(),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{
						Data: map[string][]byte{
							"key1": []byte("value1"),
						},
					},
					{
						Data: map[string][]byte{
							"key2": []byte("value2"),
						},
					},
				},
				namer: kbNamer,
			},
			want: &corev1.Secret{
				ObjectMeta: expectedMeta,
				Data: map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				},
			},
			wantErr: false,
		},
		{
			name: "multiple user secrets, key conflict, last in wins",
			args: args{
				c:           k8s.NewFakeClient(),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{
						Data: map[string][]byte{
							"key1": []byte("value1"),
						},
					},
					{
						Data: map[string][]byte{
							"key1": []byte("value2"),
						},
					},
				},
				namer: kbNamer,
			},
			want: &corev1.Secret{
				ObjectMeta: expectedMeta,
				Data: map[string][]byte{
					"key1": []byte("value2"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reconcileSecureSettings(context.Background(), tt.args.c, tt.args.hasKeystore, tt.args.userSecrets, tt.args.namer, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileSecureSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Empty(t, comparison.Diff(got, tt.want))
		})
	}
}

func Test_retrieveUserSecrets(t *testing.T) {
	testSecretName := "some-user-secret"
	testSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      testSecretName,
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key3": []byte("value3"),
		},
	}
	testKibana := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb",
			Namespace: "ns",
		},
		Spec: kbv1.KibanaSpec{
			SecureSettings: []commonv1.SecretSource{},
		},
	}

	tests := []struct {
		name    string
		args    []commonv1.SecretSource
		want    []corev1.Secret
		wantErr bool
	}{
		{
			name: "secure settings secret with only secret name should be retrieved",
			args: []commonv1.SecretSource{
				{
					SecretName: testSecretName,
				},
			},
			want:    []corev1.Secret{testSecret},
			wantErr: false,
		},
		{
			name: "secure settings secret with empty items should fail",
			args: []commonv1.SecretSource{
				{
					SecretName: testSecretName,
					Entries:    []commonv1.KeyToPath{},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secure settings secret with invalid key should fail",
			args: []commonv1.SecretSource{
				{
					SecretName: testSecretName,
					Entries: []commonv1.KeyToPath{
						{Key: "unknown"},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "secure settings secret with valid key should be retrieved",
			args: []commonv1.SecretSource{
				{
					SecretName: testSecretName,
					Entries: []commonv1.KeyToPath{
						{Key: "key2"},
					},
				},
			},
			want: []corev1.Secret{{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      testSecretName,
				},
				Data: map[string][]byte{
					"key2": []byte("value2"),
				},
			}},
			wantErr: false,
		},
		{
			name: "secure settings secret with valid key and path should be retrieved",
			args: []commonv1.SecretSource{
				{
					SecretName: testSecretName,
					Entries: []commonv1.KeyToPath{
						{Key: "key1"},
						{Key: "key3", Path: "newKey"},
					},
				},
			},
			want: []corev1.Secret{{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      testSecretName,
				},
				Data: map[string][]byte{
					"key1":   []byte("value1"),
					"newKey": []byte("value3"),
				},
			}},
			wantErr: false,
		},
	}

	recorder := record.NewFakeRecorder(100)
	client := k8s.NewFakeClient(&testSecret)
	hasKeystore := testKibana

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasKeystore.Spec.SecureSettings = tt.args
			got, err := retrieveUserSecrets(context.Background(), client, recorder, hasKeystore, WatchedSecretNames(hasKeystore))
			if (err != nil) != tt.wantErr {
				t.Errorf("retrieveUserSecrets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, len(tt.want), len(got))
			for i := range tt.want {
				comparison.AssertEqual(t, &tt.want[i], &got[i])
			}
		})
	}
}
