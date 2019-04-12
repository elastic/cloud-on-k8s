// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"fmt"
	"path"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testProbeUser = client.UserAuth{Name: "username1", Password: "supersecure"}
var testReloadCredsUser = client.UserAuth{Name: "username2", Password: "supersecure"}
var testObjectMeta = metav1.ObjectMeta{
	Name:      "my-es",
	Namespace: "default",
}

func TestNewEnvironmentVars(t *testing.T) {
	type args struct {
		p                      pod.NewPodSpecParams
		nodeCertificatesVolume volume.SecretVolume
		privateKeyVolume       volume.SecretVolume
		reloadCredsUserVolume  volume.SecretVolume
		keystoreVolume         volume.SecretVolume
	}
	tests := []struct {
		name    string
		args    args
		wantEnv []corev1.EnvVar
	}{
		{
			name: "2 nodes",
			args: args{
				p: pod.NewPodSpecParams{
					ProbeUser:       testProbeUser,
					ReloadCredsUser: testReloadCredsUser,
					Version:         "6",
				},
				nodeCertificatesVolume: volume.NewSecretVolumeWithMountPath("certs", "/certs", "/certs"),
				privateKeyVolume:       volume.NewSecretVolumeWithMountPath("key", "/key", "/key"),
				reloadCredsUserVolume:  volume.NewSecretVolumeWithMountPath("creds", "/creds", "/creds"),
				keystoreVolume:         volume.NewSecretVolumeWithMountPath("keystore", "/keystore", "/keystore"),
			},
			wantEnv: []corev1.EnvVar{
				{Name: settings.EnvPodName, Value: "", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
				}},
				{Name: settings.EnvPodIP, Value: "", ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
				}},
				{Name: settings.EnvEsJavaOpts, Value: fmt.Sprintf("-Xms%dM -Xmx%dM -Djava.security.properties=%s", 512, 512, version.SecurityPropsFile)},
				{Name: settings.EnvReadinessProbeProtocol, Value: "https"},
				{Name: settings.EnvProbeUsername, Value: "username1"},
				{Name: settings.EnvProbePasswordFile, Value: path.Join(volume.ProbeUserSecretMountPath, "username1")},
				{Name: processmanager.EnvProcName, Value: "es"},
				{Name: processmanager.EnvProcCmd, Value: "/usr/local/bin/docker-entrypoint.sh"},
				{Name: processmanager.EnvTLS, Value: "true"},
				{Name: processmanager.EnvCertPath, Value: "/certs/cert.pem"},
				{Name: processmanager.EnvKeyPath, Value: "/key/node.key"},
				{Name: keystore.EnvSourceDir, Value: "/keystore"},
				{Name: keystore.EnvReloadCredentials, Value: "true"},
				{Name: keystore.EnvEsUsername, Value: "username2"},
				{Name: keystore.EnvEsPasswordFile, Value: "/creds/username2"},
				{Name: keystore.EnvEsCaCertsPath, Value: "/certs/ca.pem"},
				{Name: keystore.EnvEsEndpoint, Value: "https://127.0.0.1:9200"},
				{Name: keystore.EnvEsVersion, Value: "6"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newEnvironmentVars(tt.args.p, tt.args.nodeCertificatesVolume, tt.args.privateKeyVolume,
				tt.args.reloadCredsUserVolume, tt.args.keystoreVolume)
			assert.Equal(t, tt.wantEnv, got)
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectNodeCount(t *testing.T) {
	tests := []struct {
		name             string
		es               v1alpha1.Elasticsearch
		expectedPodCount int
	}{
		{
			name: "2 nodes es",
			es: v1alpha1.Elasticsearch{
				ObjectMeta: testObjectMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					Nodes: []v1alpha1.NodeSpec{
						{
							NodeCount: 2,
						},
					},
				},
			},
			expectedPodCount: 2,
		},
		{
			name: "1 master 2 data",
			es: v1alpha1.Elasticsearch{
				ObjectMeta: testObjectMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					Nodes: []v1alpha1.NodeSpec{
						{
							NodeCount: 1,
							Config: v1alpha1.Config{
								Data: map[string]interface{}{
									v1alpha1.NodeMaster: "true",
								},
							},
						},
						{
							NodeCount: 2,
							Config: v1alpha1.Config{
								Data: map[string]interface{}{
									v1alpha1.NodeData: "true",
								},
							},
						},
					},
				},
			},
			expectedPodCount: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podSpecs, err := ExpectedPodSpecs(
				tt.es,
				pod.NewPodSpecParams{ProbeUser: testProbeUser},
				"operator-image-dummy",
			)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPodCount, len(podSpecs))
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectPodSpec(t *testing.T) {
	es := v1alpha1.Elasticsearch{
		ObjectMeta: testObjectMeta,
		Spec: v1alpha1.ElasticsearchSpec{
			Version: "1.2.3",
			Image:   "custom-image",
			Nodes: []v1alpha1.NodeSpec{
				{
					NodeCount: 1,
					Config: v1alpha1.Config{
						Data: map[string]interface{}{
							v1alpha1.NodeMaster: "true",
						},
					},
				},
			},
		},
	}
	podSpec, err := ExpectedPodSpecs(
		es,
		pod.NewPodSpecParams{ProbeUser: testProbeUser},
		"operator-image-dummy",
	)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(podSpec))

	esPodSpec := podSpec[0].PodSpec
	assert.Equal(t, 1, len(esPodSpec.Containers))
	assert.Equal(t, 4, len(esPodSpec.InitContainers))
	assert.Equal(t, 13, len(esPodSpec.Volumes))

	esContainer := esPodSpec.Containers[0]
	assert.NotEqual(t, 0, esContainer.Env)
	// esContainer.Env actual values are tested in environment_test.go
	assert.Equal(t, "custom-image", esContainer.Image)
	assert.NotNil(t, esContainer.ReadinessProbe)
	assert.ElementsMatch(t, pod.DefaultContainerPorts, esContainer.Ports)
	// volume mounts is one less than volumes because we're not mounting the node certs secret until pod creation time
	assert.Equal(t, 14, len(esContainer.VolumeMounts))
	assert.NotEmpty(t, esContainer.ReadinessProbe.Handler.Exec.Command)
}
