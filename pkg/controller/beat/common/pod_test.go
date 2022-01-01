// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"hash"
	"hash/fnv"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_buildPodTemplate(t *testing.T) {
	type args struct {
		params       DriverParams
		initialHash  hash.Hash32
		defaultImage container.Image
	}
	type want struct {
		initContainers int
		labels         map[string]string
		annotations    map[string]string
		err            bool
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "daemonset user-provided init containers should inherit from the default main container image",
			args: args{
				initialHash: newHash("foobar"), // SHA224 for foobar is de76c3e567fca9d246f5f8d3b2e704a38c3c5e258988ab525f941db8
				params: DriverParams{
					Watches: watches.NewDynamicWatches(),
					Client:  k8s.NewFakeClient(),
					Beat: v1beta1.Beat{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "beat-name",
							Namespace: "beat-namespace",
						},
						Spec: v1beta1.BeatSpec{
							Type:    "filebeat",
							Version: "7.15.0",
							Deployment: &v1beta1.DeploymentSpec{
								PodTemplate: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										InitContainers: []corev1.Container{
											{
												Name: "user-init-container",
											},
										},
									},
								},
							},
						},
					},
				},
				defaultImage: "beats/filebeat",
			},
			want: want{
				initContainers: 1,
				labels: map[string]string{
					"beat.k8s.elastic.co/name":    "beat-name",
					"beat.k8s.elastic.co/version": "7.15.0",
					"common.k8s.elastic.co/type":  "beat",
				},
				annotations: map[string]string{
					// SHA224 should be the same as the initial one.
					"beat.k8s.elastic.co/config-hash": "3214735720",
				},
			},
		},
		{
			// The purpose of this test is to ensure that the two init containers inherit the image from the main container,
			// and that the configuration hash is updated to reflect the change in the secure settings.
			name: "deployment user-provided init containers should with a keystore",
			args: args{
				initialHash: newHash("foobar"),
				params: DriverParams{
					Watches: watches.NewDynamicWatches(),
					Client: k8s.NewFakeClient(
						// Secret maintained by the operator
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								ResourceVersion: "1", // ResourceVersion should be incremented during the reconciliation loop
								Name:            "beat-name-beat-secure-settings",
								Namespace:       "beat-namespace",
							},
							Data: map[string][]byte{"key": []byte("value1")},
						},
						// User secret
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "foo",
								Namespace: "beat-namespace",
							},
							Data: map[string][]byte{"key": []byte("value2")},
						},
					),
					Beat: v1beta1.Beat{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "beat-name",
							Namespace: "beat-namespace",
						},
						Spec: v1beta1.BeatSpec{
							Type:    "filebeat",
							Version: "7.15.0",
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "foo",
								},
							},
							Deployment: &v1beta1.DeploymentSpec{
								PodTemplate: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										InitContainers: []corev1.Container{
											{
												Name: "user-init-container",
											},
										},
									},
								},
							},
						},
					},
				},
				defaultImage: "beats/filebeat",
			},
			want: want{
				initContainers: 2,
				labels: map[string]string{
					"beat.k8s.elastic.co/name":    "beat-name",
					"beat.k8s.elastic.co/version": "7.15.0",
					"common.k8s.elastic.co/type":  "beat",
				},
				annotations: map[string]string{
					// The sum below should reflect the version of the Secret which contain the secure settings.
					"beat.k8s.elastic.co/config-hash": "4263282862",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplateSpec, err := buildPodTemplate(tt.args.params, tt.args.defaultImage, tt.args.initialHash)
			if (err != nil) != tt.want.err {
				t.Errorf("buildPodTemplate() error = %v, wantErr %v", err, tt.want.err)
				return
			}
			assertInitContainers(t, podTemplateSpec, tt.want.initContainers)
			assertConfiguration(t, podTemplateSpec)
			if !reflect.DeepEqual(tt.want.labels, podTemplateSpec.Labels) {
				t.Errorf("Labels do not match: %s", cmp.Diff(tt.want.labels, podTemplateSpec.Labels))
				return
			}
			if !reflect.DeepEqual(tt.want.annotations, podTemplateSpec.Annotations) {
				t.Errorf("Annotations do not match: %s", cmp.Diff(tt.want.annotations, podTemplateSpec.Annotations))
				return
			}
		})
	}
}

// decimal value of '0444' in octal is 292
var expectedConfigVolumeMode int32 = 292

func assertInitContainers(t *testing.T, pod corev1.PodTemplateSpec, wantInitContainers int) {
	t.Helper()
	// Validate that init container is in the PodTemplate
	assert.Len(t, pod.Spec.InitContainers, wantInitContainers)
	// Image used by the init container and by the "main" container must be the same
	assert.Equal(t, pod.Spec.Containers[0].Image, pod.Spec.InitContainers[0].Image)
}

func assertConfiguration(t *testing.T, pod corev1.PodTemplateSpec) {
	t.Helper()
	// Validate that the Pod contains a Secret as a config volume.
	var configVolume *corev1.SecretVolumeSource
	for _, vol := range pod.Spec.Volumes {
		if vol.Secret != nil && vol.Name == "config" {
			configVolume = vol.Secret
			break
		}
	}
	assert.NotNil(t, configVolume)
	// Validate the mode
	assert.NotNil(t, configVolume.DefaultMode, "default volume mode for beat configuration should not be nil")
	assert.Equal(t, expectedConfigVolumeMode, *configVolume.DefaultMode)
}

// newHash creates a hash with some initial data.
func newHash(initialData string) hash.Hash32 {
	dataHash := fnv.New32a()
	_, _ = dataHash.Write([]byte(initialData))
	return dataHash
}
