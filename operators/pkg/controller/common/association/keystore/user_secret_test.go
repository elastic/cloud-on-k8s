// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	kbvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_secureSettingsWatchName(t *testing.T) {
	require.Equal(t, "ns-name-secure-settings", secureSettingsWatchName(types.NamespacedName{Namespace: "ns", Name: "name"}))
}

func Test_secureSettingsVolume(t *testing.T) {
	expectedSecretVolume := volume.NewSecretVolumeWithMountPath(
		testSecureSettingsSecret.Name,
		kbvolume.SecureSettingsVolumeName,
		kbvolume.SecureSettingsVolumeMountPath,
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
			vol, version, err := secureSettingsVolume(tt.c, recorder, tt.w, tt.kb)
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
