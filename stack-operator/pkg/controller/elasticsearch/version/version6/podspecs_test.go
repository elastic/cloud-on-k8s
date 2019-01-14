package version6

import (
	"reflect"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/keystore"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testProbeUser = client.User{Name: "username", Password: "supersecure"}
var testObjectMeta = metav1.ObjectMeta{
	Name:      "my-es",
	Namespace: "default",
}

func TestNewEnvironmentVars(t *testing.T) {
	type args struct {
		p                      support.NewPodSpecParams
		nodeCertificatesVolume support.SecretVolume
		extraFilesSecretVolume support.SecretVolume
	}

	tests := []struct {
		name          string
		args          args
		wantEnvSubset []corev1.EnvVar
	}{
		{name: "2 nodes",
			args: args{
				p: support.NewPodSpecParams{
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
				},
				nodeCertificatesVolume: support.SecretVolume{},
				extraFilesSecretVolume: support.SecretVolume{},
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
				{Name: "xpack.license.self_generated.type", Value: "trial"},
				{Name: "xpack.security.authc.reserved_realm.enabled", Value: "false"},
				{Name: "PROBE_USERNAME", Value: "username"},
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
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectNodeCount(t *testing.T) {
	tests := []struct {
		name             string
		es               v1alpha1.ElasticsearchCluster
		expectedPodCount int
	}{
		{
			name: "2 nodes es",
			es: v1alpha1.ElasticsearchCluster{
				ObjectMeta: testObjectMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					Topologies: []v1alpha1.ElasticsearchTopologySpec{
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
			es: v1alpha1.ElasticsearchCluster{
				ObjectMeta: testObjectMeta,
				Spec: v1alpha1.ElasticsearchSpec{
					Topologies: []v1alpha1.ElasticsearchTopologySpec{
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
				support.NewPodSpecParams{ProbeUser: testProbeUser},
				support.ResourcesState{},
			)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPodCount, len(podSpecs))
		})
	}
}

func TestCreateExpectedPodSpecsReturnsCorrectPodSpec(t *testing.T) {
	es := v1alpha1.ElasticsearchCluster{
		ObjectMeta: testObjectMeta,
		Spec: v1alpha1.ElasticsearchSpec{
			Version:          "1.2.3",
			Image:            "custom-image",
			SetVMMaxMapCount: true,
			Topologies: []v1alpha1.ElasticsearchTopologySpec{
				{
					NodeCount: 1,
					NodeTypes: v1alpha1.NodeTypesSpec{Master: true},
				},
			},
		},
	}
	podSpec, err := ExpectedPodSpecs(
		es,
		support.NewPodSpecParams{ProbeUser: testProbeUser},
		support.ResourcesState{},
	)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(podSpec))

	esPodSpec := podSpec[0].PodSpec
	assert.Equal(t, 2, len(esPodSpec.Containers))
	assert.Equal(t, 3, len(esPodSpec.InitContainers))
	assert.Equal(t, 11, len(esPodSpec.Volumes))

	esContainer := esPodSpec.Containers[0]
	assert.NotEqual(t, 0, esContainer.Env)
	// esContainer.Env actual values are tested in environment_test.go
	assert.Equal(t, "custom-image", esContainer.Image)
	assert.NotNil(t, esContainer.ReadinessProbe)
	assert.ElementsMatch(t, support.DefaultContainerPorts, esContainer.Ports)
	// volume mounts is one less than volumes because we're not mounting the node certs secret until pod creation time
	assert.Equal(t, 10, len(esContainer.VolumeMounts))
	assert.NotEmpty(t, esContainer.ReadinessProbe.Handler.Exec.Command)
}

func Test_newSidecarContainers(t *testing.T) {
	type args struct {
		imageName string
		spec      support.NewPodSpecParams
		volumes   map[string]support.VolumeLike
	}
	tests := []struct {
		name    string
		args    args
		want    []corev1.Container
		wantErr bool
	}{
		{
			name:    "error: no keystore volume",
			args:    args{imageName: "test-operator-image", spec: support.NewPodSpecParams{}},
			want:    []corev1.Container{},
			wantErr: true,
		},
		{
			name: "error: no probe user volume",
			args: args{
				imageName: "test-operator-image",
				spec:      support.NewPodSpecParams{},
				volumes: map[string]support.VolumeLike{
					keystore.SecretVolumeName: support.SecretVolume{},
				},
			},
			want:    []corev1.Container{},
			wantErr: true,
		},
		{
			name: "error: no cert volume",
			args: args{
				imageName: "test-operator-image",
				spec:      support.NewPodSpecParams{},
				volumes: map[string]support.VolumeLike{
					keystore.SecretVolumeName:   support.SecretVolume{},
					support.ProbeUserVolumeName: support.SecretVolume{},
				},
			},
			want:    []corev1.Container{},
			wantErr: true,
		},
		{
			name: "success: expected container present",
			args: args{
				imageName: "test-operator-image",
				spec:      support.NewPodSpecParams{},
				volumes: map[string]support.VolumeLike{
					keystore.SecretVolumeName:                support.NewSecretVolumeWithMountPath("keystore", "keystore", "/keystore"),
					support.ProbeUserVolumeName:              support.NewSecretVolumeWithMountPath("user", "user", "/user"),
					support.NodeCertificatesSecretVolumeName: support.NewSecretVolumeWithMountPath("ca.pem", "certs", "/certs"),
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
						{Name: "USERNAME", Value: ""}, // because dummy probe user is used
						{Name: "PASSWORD_FILE", Value: "/probe-user"},
						{Name: "CERTIFICATES_PATH", Value: "/certs/ca.pem"},
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
							Name:      "user",
							ReadOnly:  true,
							MountPath: "/user",
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
