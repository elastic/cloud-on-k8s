// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version6

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
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
		reloadCredsUserVolume  volume.SecretVolume
		keystoreVolume         volume.SecretVolume
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
				nodeCertificatesVolume: volume.NewSecretVolumeWithMountPath("certs", "/certs", "/certs"),
				reloadCredsUserVolume:  volume.NewSecretVolumeWithMountPath("creds", "/creds", "/creds"),
				keystoreVolume:         volume.NewSecretVolumeWithMountPath("keystore", "/keystore", "/keystore"),
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
				{Name: processmanager.EnvProcName, Value: "es"},
				{Name: processmanager.EnvProcCmd, Value: "/usr/local/bin/docker-entrypoint.sh"},
				{Name: "KEYSTORE_SOURCE_DIR", Value: "/keystore"},
				{Name: "KEYSTORE_RELOAD_CREDENTIALS", Value: "true"},
				{Name: "KEYSTORE_ES_USERNAME", Value: "username2"},
				{Name: "KEYSTORE_ES_PASSWORD_FILE", Value: "/creds/username2"},
				{Name: "KEYSTORE_ES_CA_CERTS_PATH", Value: "/certs/ca.pem"},
				{Name: "KEYSTORE_ES_ENDPOINT", Value: "https://127.0.0.1:9200"},
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
				tt.args.p, tt.args.nodeCertificatesVolume, tt.args.reloadCredsUserVolume, tt.args.keystoreVolume, tt.args.extraFilesSecretVolume,
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
	assert.Equal(t, 1, len(esPodSpec.Containers))
	assert.Equal(t, 4, len(esPodSpec.InitContainers))
	assert.Equal(t, 12, len(esPodSpec.Volumes))

	esContainer := esPodSpec.Containers[0]
	assert.NotEqual(t, 0, esContainer.Env)
	// esContainer.Env actual values are tested in environment_test.go
	assert.Equal(t, "custom-image", esContainer.Image)
	assert.NotNil(t, esContainer.ReadinessProbe)
	assert.ElementsMatch(t, pod.DefaultContainerPorts, esContainer.Ports)
	// volume mounts is one less than volumes because we're not mounting the node certs secret until pod creation time
	assert.Equal(t, 13, len(esContainer.VolumeMounts))
	assert.NotEmpty(t, esContainer.ReadinessProbe.Handler.Exec.Command)
}
