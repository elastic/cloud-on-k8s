// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"
	"testing"

	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	watches2 "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
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
			Kind: kbv1.Kind,
		},
		ObjectMeta: testKibana.ObjectMeta,
		Spec: kbv1.KibanaSpec{
			SecureSettings: []commonv1.SecretSource{testSecureSettingsSecretRef},
		},
	}

	testResourceRequirements = corev1.ResourceRequirements{
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}
)

func fakeFlagInitContainersParameters(skipInitializedFlag bool) InitContainerParameters {
	return InitContainerParameters{
		KeystoreCreateCommand:         "/keystore/bin/keystore create",
		KeystoreAddCommand:            `/keystore/bin/keystore add "$key" "$filename"`,
		SecureSettingsVolumeMountPath: "/foo/secret",
		KeystoreVolumePath:            "/bar/data",
		Resources:                     testResourceRequirements,
		SkipInitializedFlag:           skipInitializedFlag,
	}
}

func wantContainer(wantScript string) *corev1.Container {
	varFalse := false
	return &corev1.Container{
		Command: []string{
			"/usr/bin/env",
			"bash",
			"-c",
			wantScript,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "elastic-internal-secure-settings",
				ReadOnly:  true,
				MountPath: "/mnt/elastic-internal/secure-settings",
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &varFalse,
		},
		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
	}
}

func TestReconcileResources(t *testing.T) {
	tests := []struct {
		name                    string
		client                  k8s.Client
		kb                      kbv1.Kibana
		initContainerParameters InitContainerParameters
		wantNil                 bool
		wantContainers          *corev1.Container
		wantHash                string
	}{
		{
			name:                    "no secure settings specified: no resources",
			client:                  k8s.NewFakeClient(),
			kb:                      testKibana,
			initContainerParameters: fakeFlagInitContainersParameters(false),
			wantContainers:          nil,
			wantHash:                "",
			wantNil:                 true,
		},
		{
			name:                    "secure settings specified: return volume, init container and (empty) version",
			client:                  k8s.NewFakeClient(&testSecureSettingsSecret),
			kb:                      testKibanaWithSecureSettings,
			initContainerParameters: fakeFlagInitContainersParameters(false),
			wantContainers: wantContainer(`#!/usr/bin/env bash

set -eux

keystore_initialized_flag=/bar/data/elastic-internal-init-keystore.ok

if [[ -f "${keystore_initialized_flag}" ]]; then
    echo "Keystore already initialized."
	exit 0
fi

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

touch /bar/data/elastic-internal-init-keystore.ok
echo "Keystore initialization successful."
`),
			// since this will be created, it will be incremented
			wantHash: "896069204",
			wantNil:  false,
		},
		{
			name:                    "Skip create keystore flag",
			client:                  k8s.NewFakeClient(&testSecureSettingsSecret),
			initContainerParameters: fakeFlagInitContainersParameters(true),
			kb:                      testKibanaWithSecureSettings,
			wantContainers: wantContainer(`#!/usr/bin/env bash

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
`),
			// since this will be created, it will be incremented
			wantHash: "896069204",
			wantNil:  false,
		},
		{
			name:           "secure settings specified but secret not there: no resources",
			client:         k8s.NewFakeClient(),
			kb:             testKibanaWithSecureSettings,
			wantContainers: nil,
			wantHash:       "",
			wantNil:        true,
		},
		{
			name:   "use custom script",
			client: k8s.NewFakeClient(&testSecureSettingsSecret),
			kb:     testKibanaWithSecureSettings,
			initContainerParameters: InitContainerParameters{
				CustomScript:        `echo "custom script"`,
				Resources:           testResourceRequirements,
				SkipInitializedFlag: false,
			},
			wantContainers: wantContainer(`echo "custom script"`),
			// since this will be created, it will be incremented
			wantHash: "896069204",
			wantNil:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDriver := driver.TestDriver{
				Client:       tt.client,
				Watches:      watches2.NewDynamicWatches(),
				FakeRecorder: record.NewFakeRecorder(1000),
			}
			resources, err := ReconcileResources(context.Background(), testDriver, &tt.kb, kbNamer, nil, tt.initContainerParameters)
			require.NoError(t, err)
			if tt.wantNil {
				require.Nil(t, resources)
			} else {
				require.NotNil(t, resources)
				assert.Equal(t, resources.InitContainer.Name, "elastic-internal-init-keystore")
				assert.Equal(t, resources.InitContainer.Command, tt.wantContainers.Command)
				assert.Equal(t, resources.InitContainer.VolumeMounts, tt.wantContainers.VolumeMounts)
				assert.Equal(t, resources.InitContainer.SecurityContext, tt.wantContainers.SecurityContext)
				assert.Equal(t, resources.InitContainer.Resources, tt.wantContainers.Resources)
				assert.Equal(t, resources.Hash, tt.wantHash)
			}
		})
	}
}
