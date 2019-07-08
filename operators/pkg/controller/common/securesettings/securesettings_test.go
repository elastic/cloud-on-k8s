// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	"reflect"
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	watches2 "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	kbvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	testSecureSettingsSecretName = "secure-settings-secret"
	testSecureSettingsSecret     = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "namespace",
			Name:            testSecureSettingsSecretName,
			ResourceVersion: "resource-version",
		},
	}
	testSecureSettingsSecretRef = commonv1alpha1.SecretRef{
		SecretName: testSecureSettingsSecretName,
	}
	testKibana = v1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "kibana",
		},
	}
	testKibanaWithSecureSettings = v1alpha1.Kibana{
		ObjectMeta: testKibana.ObjectMeta,
		Spec: v1alpha1.KibanaSpec{
			SecureSettings: &testSecureSettingsSecretRef,
		},
	}
)

func TestResources(t *testing.T) {
	tests := []struct {
		name           string
		client         k8s.Client
		kb             v1alpha1.Kibana
		wantVolumes    int
		wantContainers int
		wantVersion    string
	}{
		{
			name:           "no secure settings specified: no resources",
			client:         k8s.WrapClient(fake.NewFakeClient()),
			kb:             v1alpha1.Kibana{},
			wantVolumes:    0,
			wantContainers: 0,
			wantVersion:    "",
		},
		{
			name:           "secure settings specified: return volume, init container and version",
			client:         k8s.WrapClient(fake.NewFakeClient(&testSecureSettingsSecret)),
			kb:             testKibanaWithSecureSettings,
			wantVolumes:    1,
			wantContainers: 1,
			wantVersion:    testSecureSettingsSecret.ResourceVersion,
		},
		{
			name:           "secure settings specified but secret not there: no resources",
			client:         k8s.WrapClient(fake.NewFakeClient()),
			kb:             testKibanaWithSecureSettings,
			wantVolumes:    0,
			wantContainers: 0,
			wantVersion:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := record.NewFakeRecorder(1000)
			watches := watches2.NewDynamicWatches()
			require.NoError(t, watches.InjectScheme(scheme.Scheme))
			wantKeystoreUpdater, err := Resources(
				tt.client,
				recorder,
				watches,
				"kibana-keystore",
				&tt.kb,
				k8s.ExtractNamespacedName(&tt.kb),
				tt.kb.Spec.SecureSettings,
				kbvolume.SecureSettingsVolumeName,
				kbvolume.SecureSettingsVolumeMountPath,
				kbvolume.KibanaDataVolume.VolumeMount(),
			)
			require.NoError(t, err)
			wantVolumes := []corev1.Volume{wantKeystoreUpdater.Volume}
			if !reflect.DeepEqual(len(wantVolumes), tt.wantVolumes) {
				t.Errorf("Resources() got = %v, want %v", wantVolumes, tt.wantVolumes)
			}
			wantContainers := []corev1.Container{wantKeystoreUpdater.InitContainer}
			if !reflect.DeepEqual(len(wantContainers), tt.wantContainers) {
				t.Errorf("Resources() got1 = %v, want %v", wantContainers, tt.wantContainers)
			}
			wantVersion := wantKeystoreUpdater.Version
			if wantVersion != tt.wantVersion {
				t.Errorf("Resources() got2 = %v, want %v", wantVersion, tt.wantVersion)
			}
		})
	}
}
