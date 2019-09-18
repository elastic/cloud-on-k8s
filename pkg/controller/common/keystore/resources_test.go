// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	watches2 "github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	initContainersParameters = InitContainerParameters{
		KeystoreCreateCommand:         "/keystore/bin/keystore create",
		KeystoreAddCommand:            `/keystore/bin/keystore add "$key" "$filename"`,
		SecureSettingsVolumeMountPath: "/foo/secret",
		DataVolumePath:                "/bar/data",
	}

	testSecureSettingsSecretName = "secure-settings-secret"
	testSecureSettingsSecret     = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      testSecureSettingsSecretName,
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
		},
	}
	testSecureSettingsSecretRef = commonv1alpha1.SecretSource{
		SecretName: testSecureSettingsSecretName,
	}
	testKibana = v1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "kibana",
		},
	}
	testKibanaWithSecureSettings = v1alpha1.Kibana{
		TypeMeta: metav1.TypeMeta{
			Kind: "kibana",
		},
		ObjectMeta: testKibana.ObjectMeta,
		Spec: v1alpha1.KibanaSpec{
			SecureSettings: []commonv1alpha1.SecretSource{testSecureSettingsSecretRef},
		},
	}
)

func TestResources(t *testing.T) {
	sc := scheme.Scheme
	require.NoError(t, v1alpha1.SchemeBuilder.AddToScheme(sc))

	varFalse := false
	tests := []struct {
		name           string
		client         k8s.Client
		kb             v1alpha1.Kibana
		wantNil        bool
		wantContainers *corev1.Container
		wantVersion    string
	}{
		{
			name:           "no secure settings specified: no resources",
			client:         k8s.WrapClient(fake.NewFakeClient()),
			kb:             testKibana,
			wantContainers: nil,
			wantVersion:    "",
			wantNil:        true,
		},
		{
			name:   "secure settings specified: return volume, init container and (empty) version",
			client: k8s.WrapClient(fake.NewFakeClient(&testSecureSettingsSecret)),
			kb:     testKibanaWithSecureSettings,
			wantContainers: &corev1.Container{
				Command: []string{
					"/usr/bin/env",
					"bash",
					"-c",
					`#!/usr/bin/env bash

set -eux

echo "Initializing keystore."

# create a keystore in the default data path
/keystore/bin/keystore create

# add all existing secret entries into it
for filename in  /foo/secret/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	/keystore/bin/keystore add "$key" "$filename"
done

echo "Keystore initialization successful."
`,
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "elastic-internal-secure-settings",
						ReadOnly:  true,
						MountPath: "/mnt/elastic-internal/secure-settings",
					},
					{
						Name:      "kibana-data",
						ReadOnly:  false,
						MountPath: "/bar/data",
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: &varFalse,
				},
			},
			wantVersion: testSecureSettingsSecret.ResourceVersion,
			wantNil:     false,
		},
		{
			name:           "secure settings specified but secret not there: no resources",
			client:         k8s.WrapClient(fake.NewFakeClient()),
			kb:             testKibanaWithSecureSettings,
			wantContainers: nil,
			wantVersion:    "",
			wantNil:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDriver := driver.TestDriver{
				Client:        tt.client,
				RuntimeScheme: scheme.Scheme,
				Watches:       watches2.NewDynamicWatches(),
				FakeRecorder:  record.NewFakeRecorder(1000),
			}
			require.NoError(t, testDriver.Watches.InjectScheme(scheme.Scheme))
			resources, err := NewResources(testDriver, &tt.kb, name.KBNamer, nil, initContainersParameters)
			require.NoError(t, err)
			if tt.wantNil {
				require.Nil(t, resources)
			} else {
				require.NotNil(t, resources)
				assert.Equal(t, resources.InitContainer.Name, "elastic-internal-init-keystore")
				assert.Equal(t, resources.InitContainer.Command, tt.wantContainers.Command)
				assert.Equal(t, resources.InitContainer.VolumeMounts, tt.wantContainers.VolumeMounts)
				assert.Equal(t, resources.InitContainer.SecurityContext, tt.wantContainers.SecurityContext)
				assert.Equal(t, resources.Version, tt.wantVersion)
			}

		})
	}
}
