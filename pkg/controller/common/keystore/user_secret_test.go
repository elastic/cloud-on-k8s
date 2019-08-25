// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	kbname "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_secureSettingsWatchName(t *testing.T) {
	require.Equal(t, "ns-name-secure-settings", secureSettingsWatchName(types.NamespacedName{Namespace: "ns", Name: "name"}))
}

func Test_secureSettingsVolume(t *testing.T) {
	s := scheme.Scheme
	require.NoError(t, v1alpha1.AddToScheme(s))

	expectedSecretVolume := volume.NewSecretVolumeWithMountPath(
		"kibana-kb-secure-settings",
		SecureSettingsVolumeName,
		SecureSettingsVolumeMountPath,
	)
	createWatches := func(handlerName string) watches.DynamicWatches {
		w := watches.NewDynamicWatches()
		require.NoError(t, w.InjectScheme(scheme.Scheme))
		if handlerName != "" {
			require.NoError(t, w.Secrets.AddHandler(watches.NamedWatch{
				Name: handlerName,
			}))
		}
		return w
	}
	tests := []struct {
		name        string
		c           k8s.Client
		w           watches.DynamicWatches
		kb          v1alpha1.Kibana
		wantVolume  *volume.SecretVolume
		wantVersion string
		wantWatches []string
		wantEvent   string
	}{
		{
			name:        "no secure settings specified in Kibana spec: should return nothing",
			c:           k8s.WrapClient(fake.NewFakeClient()),
			w:           createWatches(""),
			kb:          v1alpha1.Kibana{},
			wantVolume:  nil,
			wantVersion: "",
			wantWatches: []string{},
		},
		{
			name:        "valid secure settings specified: should add watch and return volume with version",
			c:           k8s.WrapClient(fake.NewFakeClient(&testSecureSettingsSecret)),
			w:           createWatches(""),
			kb:          testKibanaWithSecureSettings,
			wantVolume:  &expectedSecretVolume,
			wantVersion: testSecureSettingsSecret.ResourceVersion,
			wantWatches: []string{secureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))},
		},
		{
			name:        "secure setting specified but no secret exists: should return nothing but watch the secret, and emit an event",
			c:           k8s.WrapClient(fake.NewFakeClient()),
			w:           createWatches(secureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))),
			kb:          testKibanaWithSecureSettings,
			wantVolume:  nil,
			wantVersion: "",
			wantWatches: []string{secureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))},
			wantEvent:   "Warning Unexpected Secure settings secret not found: secure-settings-secret",
		},
		{
			name:        "secure settings removed (was set before): should remove watch",
			c:           k8s.WrapClient(fake.NewFakeClient(&testSecureSettingsSecret)),
			w:           createWatches(secureSettingsWatchName(k8s.ExtractNamespacedName(&testKibanaWithSecureSettings))),
			kb:          testKibana,
			wantVolume:  nil,
			wantVersion: "",
			wantWatches: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := record.NewFakeRecorder(1000)
			vol, version, err := secureSettingsVolume(tt.c, scheme.Scheme, recorder, tt.w, &tt.kb, nil, kbname.KBNamer)
			require.NoError(t, err)

			if !reflect.DeepEqual(vol, tt.wantVolume) {
				t.Errorf("secureSettingsVolume() got = %v, want %v", vol, tt.wantVolume)
			}
			if version != tt.wantVersion {
				t.Errorf("secureSettingsVolume() got1 = %v, want %v", version, tt.wantVersion)
			}

			require.Equal(t, tt.wantWatches, tt.w.Secrets.Registrations())

			if tt.wantEvent != "" {
				require.Equal(t, tt.wantEvent, <-recorder.Events)
			} else {
				// no event expected
				select {
				case e := <-recorder.Events:
					require.Fail(t, "no event expected but got one", "event", e)
				default:
					// ok
				}
			}
		})
	}
}

func Test_reconcileSecureSettings(t *testing.T) {
	true := true
	s := scheme.Scheme
	require.NoError(t, v1alpha1.AddToScheme(s))

	type args struct {
		c           k8s.Client
		hasKeystore HasKeystore
		userSecrets []corev1.Secret
		namer       name.Namer
	}
	kibanaFixture := &v1alpha1.Kibana{
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
				APIVersion:         "kibana.k8s.elastic.co/v1alpha1",
				Kind:               "Kibana",
				Name:               "kb",
				UID:                "",
				Controller:         &true,
				BlockOwnerDeletion: &true,
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
				c:           k8s.WrapClient(fake.NewFakeClient()),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{},
				},
				namer: kbname.KBNamer,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "new user secret",
			args: args{
				c:           k8s.WrapClient(fake.NewFakeClient()),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{
						Data: map[string][]byte{
							"key1": []byte("value1"),
						}},
				},
				namer: kbname.KBNamer,
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
				c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
					ObjectMeta: expectedMeta,
					Data: map[string][]byte{
						"key1": []byte("old-value"),
					},
				})),
				hasKeystore: kibanaFixture,
				userSecrets: []corev1.Secret{
					{
						Data: map[string][]byte{
							"key1": []byte("value1"),
						},
					},
				},
				namer: kbname.KBNamer,
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
				c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
					ObjectMeta: expectedMeta,
					Data: map[string][]byte{
						"key1": []byte("value1"),
					},
				})),
				hasKeystore: kibanaFixture,
				userSecrets: nil,
				namer:       kbname.KBNamer,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "no secure settings and no previous settings",
			args: args{
				c:           k8s.WrapClient(fake.NewFakeClient()),
				hasKeystore: kibanaFixture,
				userSecrets: nil,
				namer:       kbname.KBNamer,
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "multiple user secrets",
			args: args{
				c:           k8s.WrapClient(fake.NewFakeClient()),
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
				namer: kbname.KBNamer,
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
				c:           k8s.WrapClient(fake.NewFakeClient()),
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
				namer: kbname.KBNamer,
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
			got, err := reconcileSecureSettings(tt.args.c, s, tt.args.hasKeystore, tt.args.userSecrets, tt.args.namer, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileSecureSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Nil(t, deep.Equal(got, tt.want))
		})
	}
}
