// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
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
		extraFilesSecretVolume volume.SecretVolume
	}

	tests := []struct {
		name              string
		args              args
		wantEnvSubset     []corev1.EnvVar
		dontWantEnvSubset []corev1.EnvVar
	}{
		{
			name: "2 nodes",
			args: args{
				p: pod.NewPodSpecParams{
					ClusterName:                    "cluster",
					CustomImageName:                "myImage",
					DiscoveryServiceName:           "discovery-service",
					DiscoveryZenMinimumMasterNodes: 3,
					NodeTypes: v1alpha1.NodeTypesSpec{
						Master: true,
						Data:   true,
						Ingest: false,
						ML:     true,
					},
					SetVMMaxMapCount: true,
					Version:          "1.2.3",
					ProbeUser:        testProbeUser,
					ReloadCredsUser:  testReloadCredsUser,
				},
				nodeCertificatesVolume: volume.SecretVolume{},
				extraFilesSecretVolume: volume.SecretVolume{},
			},
			wantEnvSubset: []corev1.EnvVar{
				{Name: "discovery.zen.ping.unicast.hosts", Value: "discovery-service"},
				{Name: "cluster.name", Value: "cluster"},
				{Name: "discovery.zen.minimum_master_nodes", Value: "3"},
				{Name: "network.host", Value: "0.0.0.0"},
				{Name: "path.data", Value: "/usr/share/elasticsearch/data"},
				{Name: "path.logs", Value: "/usr/share/elasticsearch/logs"},
				{Name: "ES_JAVA_OPTS", Value: "-Xms512M -Xmx512M -Djava.security.properties=/usr/share/elasticsearch/config/managed/security.properties"},
				{Name: "node.master", Value: "true"},
				{Name: "node.data", Value: "true"},
				{Name: "node.ingest", Value: "false"},
				{Name: "node.ml", Value: "true"},
				{Name: "xpack.security.enabled", Value: "true"},
				{Name: "xpack.security.authc.reserved_realm.enabled", Value: "false"},
				{Name: "PROBE_USERNAME", Value: "username1"},
				{Name: "READINESS_PROBE_PROTOCOL", Value: "https"},
			},
		},
		{
			name: "trial license",
			args: args{
				p: pod.NewPodSpecParams{
					ClusterName:                    "cluster",
					CustomImageName:                "myImage",
					DiscoveryServiceName:           "discovery-service",
					DiscoveryZenMinimumMasterNodes: 3,
					LicenseType:                    v1alpha1.LicenseTypeTrial,
					NodeTypes: v1alpha1.NodeTypesSpec{
						Master: true,
						Data:   true,
						Ingest: false,
						ML:     true,
					},
					SetVMMaxMapCount: true,
					Version:          "1.2.3",
					ProbeUser:        testProbeUser,
					ReloadCredsUser:  testReloadCredsUser,
				},
				nodeCertificatesVolume: volume.SecretVolume{},
				extraFilesSecretVolume: volume.SecretVolume{},
			},
			wantEnvSubset: []corev1.EnvVar{
				{Name: "xpack.license.self_generated.type", Value: "trial"},
				{Name: "xpack.security.enabled", Value: "true"},
				{Name: "READINESS_PROBE_PROTOCOL", Value: "https"},
			},
		},
		{
			name: "basic license",
			args: args{
				p: pod.NewPodSpecParams{
					ClusterName:                    "cluster",
					CustomImageName:                "myImage",
					DiscoveryServiceName:           "discovery-service",
					DiscoveryZenMinimumMasterNodes: 3,
					LicenseType:                    v1alpha1.LicenseTypeBasic,
					NodeTypes: v1alpha1.NodeTypesSpec{
						Master: true,
						Data:   true,
						Ingest: false,
						ML:     true,
					},
					SetVMMaxMapCount: true,
					Version:          "1.2.3",
					ProbeUser:        testProbeUser,
					ReloadCredsUser:  testReloadCredsUser,
				},
				nodeCertificatesVolume: volume.SecretVolume{},
				extraFilesSecretVolume: volume.SecretVolume{},
			},
			wantEnvSubset: []corev1.EnvVar{
				{Name: "READINESS_PROBE_PROTOCOL", Value: "http"},
				{Name: "xpack.security.enabled", Value: "false"},
			},
			dontWantEnvSubset: []corev1.EnvVar{
				{Name: "xpack.license.self_generated.type", Value: "trial"},
			},
		},
		{
			name: "gold license",
			args: args{
				p: pod.NewPodSpecParams{
					ClusterName:                    "cluster",
					CustomImageName:                "myImage",
					DiscoveryServiceName:           "discovery-service",
					DiscoveryZenMinimumMasterNodes: 3,
					LicenseType:                    v1alpha1.LicenseTypeGold,
					NodeTypes: v1alpha1.NodeTypesSpec{
						Master: true,
						Data:   true,
						Ingest: false,
						ML:     true,
					},
					SetVMMaxMapCount: true,
					Version:          "1.2.3",
					ProbeUser:        testProbeUser,
					ReloadCredsUser:  testReloadCredsUser,
				},
				nodeCertificatesVolume: volume.SecretVolume{},
				extraFilesSecretVolume: volume.SecretVolume{},
			},
			wantEnvSubset: []corev1.EnvVar{
				{Name: "READINESS_PROBE_PROTOCOL", Value: "https"},
				{Name: "xpack.security.enabled", Value: "true"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newEnvironmentVars(
				tt.args.p, tt.args.nodeCertificatesVolume, tt.args.extraFilesSecretVolume,
			)
			for _, v := range tt.wantEnvSubset {
				assert.Contains(t, got, v)
			}
			for _, v := range tt.dontWantEnvSubset {
				assert.NotContains(t, got, v)
			}
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
					Topology: []v1alpha1.TopologyElementSpec{
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
					Topology: []v1alpha1.TopologyElementSpec{
						{
							NodeCount: 1,
							NodeTypes: v1alpha1.NodeTypesSpec{Master: true},
						},
						{
							NodeCount: 2,
							NodeTypes: v1alpha1.NodeTypesSpec{Data: true},
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
			Version:          "1.2.3",
			Image:            "custom-image",
			SetVMMaxMapCount: true,
			Topology: []v1alpha1.TopologyElementSpec{
				{
					NodeCount: 1,
					NodeTypes: v1alpha1.NodeTypesSpec{Master: true},
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
	assert.Equal(t, 2, len(esPodSpec.Containers))
	assert.Equal(t, 4, len(esPodSpec.InitContainers))
	assert.Equal(t, 13, len(esPodSpec.Volumes))

	esContainer := esPodSpec.Containers[0]
	assert.NotEqual(t, 0, esContainer.Env)
	// esContainer.Env actual values are tested in environment_test.go
	assert.Equal(t, "custom-image", esContainer.Image)
	assert.NotNil(t, esContainer.ReadinessProbe)
	assert.ElementsMatch(t, pod.DefaultContainerPorts, esContainer.Ports)
	// volume mounts is one less than volumes because we're not mounting the node certs secret until pod creation time
	assert.Equal(t, 11, len(esContainer.VolumeMounts))
	assert.NotEmpty(t, esContainer.ReadinessProbe.Handler.Exec.Command)
}

func Test_newSidecarContainers(t *testing.T) {
	type args struct {
		imageName string
		spec      pod.NewPodSpecParams
		volumes   map[string]volume.VolumeLike
	}
	tests := []struct {
		name    string
		args    args
		want    []corev1.Container
		wantErr bool
	}{
		{
			name:    "error: no keystore volume",
			args:    args{imageName: "test-operator-image", spec: pod.NewPodSpecParams{}},
			wantErr: true,
		},
		{
			name: "error: no reload creds user volume",
			args: args{
				imageName: "test-operator-image",
				spec:      pod.NewPodSpecParams{},
				volumes: map[string]volume.VolumeLike{
					keystore.SecretVolumeName: volume.SecretVolume{},
				},
			},
			wantErr: true,
		},
		{
			name: "error: no cert volume",
			args: args{
				imageName: "test-operator-image",
				spec:      pod.NewPodSpecParams{},
				volumes: map[string]volume.VolumeLike{
					keystore.SecretVolumeName:        volume.SecretVolume{},
					volume.ReloadCredsUserVolumeName: volume.SecretVolume{},
				},
			},
			wantErr: true,
		},
		{
			name: "success: expected container present",
			args: args{
				imageName: "test-operator-image",
				spec: pod.NewPodSpecParams{
					ReloadCredsUser: testReloadCredsUser,
				},
				volumes: map[string]volume.VolumeLike{
					keystore.SecretVolumeName:               volume.NewSecretVolumeWithMountPath("keystore", "keystore", "/keystore"),
					volume.ReloadCredsUserVolumeName:        volume.NewSecretVolumeWithMountPath("users", "users", "/mnt/elastic/reload-creds-user"),
					volume.NodeCertificatesSecretVolumeName: volume.NewSecretVolumeWithMountPath("ca.pem", "certs", "/certs"),
				},
			},
			want: []corev1.Container{
				{
					Name:            "keystore-updater",
					Image:           "test-operator-image",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"/opt/sidecar/bin/keystore-updater"},
					Env: []corev1.EnvVar{
						{Name: "SOURCE_DIR", Value: "/keystore"},
						{Name: "RELOAD_CREDENTIALS", Value: "true"},
						{Name: "USERNAME", Value: "username2"},
						{Name: "PASSWORD_FILE", Value: "/mnt/elastic/reload-creds-user/username2"},
						{Name: "CERTIFICATES_PATH", Value: "/certs/ca.pem"},
						{Name: "ENDPOINT", Value: "https://127.0.0.1:9200"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config-volume",
							MountPath: "/usr/share/elasticsearch/config",
						},
						{
							Name:      "plugins-volume",
							MountPath: "/usr/share/elasticsearch/plugins",
						},
						{
							Name:      "bin-volume",
							MountPath: "/usr/share/elasticsearch/bin",
						},
						{
							Name:      "data",
							MountPath: "/usr/share/elasticsearch/data",
						},
						{
							Name:      "logs",
							MountPath: "/usr/share/elasticsearch/logs",
						},
						{
							Name:      "sidecar-bin",
							MountPath: "/opt/sidecar/bin",
						},
						{
							Name:      "certs",
							ReadOnly:  true,
							MountPath: "/certs",
						},
						{
							Name:      "keystore",
							ReadOnly:  true,
							MountPath: "/keystore",
						},
						{
							Name:      "users",
							ReadOnly:  true,
							MountPath: "/mnt/elastic/reload-creds-user",
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: defaultSidecarMemoryLimits,
							corev1.ResourceCPU:    defaultSidecarCPULimits,
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newSidecarContainers(tt.args.imageName, tt.args.spec, tt.args.volumes)
			if (err != nil) != tt.wantErr {
				t.Errorf("newSidecarContainers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newSidecarContainers() = %v, want %v", got, tt.want)
			}
		})
	}
}
