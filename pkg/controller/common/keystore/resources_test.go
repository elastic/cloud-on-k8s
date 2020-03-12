// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	watches2 "github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
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
	testSecureSettingsSecretRef = commonv1.SecretSource{
		SecretName: testSecureSettingsSecretName,
	}
	testKibana = kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "kibana",
		},
	}
	testKibanaWithSecureSettings = kbv1.Kibana{
		TypeMeta: metav1.TypeMeta{
			Kind: "kibana",
		},
		ObjectMeta: testKibana.ObjectMeta,
		Spec: kbv1.KibanaSpec{
			SecureSettings: []commonv1.SecretSource{testSecureSettingsSecretRef},
		},
	}
)

func TestResources(t *testing.T) {
	varFalse := false
	tests := []struct {
		name           string
		client         k8s.Client
		kb             kbv1.Kibana
		wantNil        bool
		wantContainers *corev1.Container
		wantVersion    string
	}{
		{
			name:           "no secure settings specified: no resources",
			client:         k8s.WrappedFakeClient(),
			kb:             testKibana,
			wantContainers: nil,
			wantVersion:    "",
			wantNil:        true,
		},
		{
			name:   "secure settings specified: return volume, init container and (empty) version",
			client: k8s.WrappedFakeClient(&testSecureSettingsSecret),
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
			// since this will be created, it will be incremented
			wantVersion: "1",
			wantNil:     false,
		},
		{
			name:           "secure settings specified but secret not there: no resources",
			client:         k8s.WrappedFakeClient(),
			kb:             testKibanaWithSecureSettings,
			wantContainers: nil,
			wantVersion:    "",
			wantNil:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDriver := driver.TestDriver{
				Client:       tt.client,
				Watches:      watches2.NewDynamicWatches(),
				FakeRecorder: record.NewFakeRecorder(1000),
			}
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
