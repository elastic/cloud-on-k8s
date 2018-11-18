package elasticsearch

import (
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNewEnvironmentVars(t *testing.T) {
	type args struct {
		p                      NewPodSpecParams
		dataVolume             EmptyDirVolume
		logsVolume             EmptyDirVolume
		probeUser              client.User
		extraFilesSecretVolume SecretVolume
	}
	tests := []struct {
		name          string
		args          args
		wantEnvSubset []corev1.EnvVar
	}{
		{name: "2 nodes",
			args: args{
				p: NewPodSpecParams{
					ClusterName:                    "cluster",
					CustomImageName:                "myImage",
					DiscoveryServiceName:           "discovery-service",
					DiscoveryZenMinimumMasterNodes: 3,
					NodeTypes: deploymentsv1alpha1.NodeTypesSpec{
						Master: true,
						Data:   true,
						Ingest: false,
						ML:     true,
					},
					SetVMMaxMapCount: true,
					Version:          "1.2.3",
				},
				dataVolume: EmptyDirVolume{
					name:      "data",
					mountPath: "/mnt/data",
				},
				logsVolume: EmptyDirVolume{
					name:      "logs",
					mountPath: "/mnt/logs",
				},
				probeUser:              client.User{Name: "name", Password: "zupersecure"},
				extraFilesSecretVolume: SecretVolume{},
			},
			wantEnvSubset: []corev1.EnvVar{
				corev1.EnvVar{Name: "discovery.zen.ping.unicast.hosts", Value: "discovery-service"},
				corev1.EnvVar{Name: "cluster.name", Value: "cluster"},
				corev1.EnvVar{Name: "discovery.zen.minimum_master_nodes", Value: "3"},
				corev1.EnvVar{Name: "network.host", Value: "0.0.0.0"},
				corev1.EnvVar{Name: "path.data", Value: "/mnt/data"},
				corev1.EnvVar{Name: "path.logs", Value: "/mnt/logs"},
				corev1.EnvVar{Name: "ES_JAVA_OPTS", Value: "-Xms1g -Xmx1g"},
				corev1.EnvVar{Name: "node.master", Value: "true"},
				corev1.EnvVar{Name: "node.data", Value: "true"},
				corev1.EnvVar{Name: "node.ingest", Value: "false"},
				corev1.EnvVar{Name: "node.ml", Value: "true"},
				corev1.EnvVar{Name: "xpack.security.enabled", Value: "true"},
				corev1.EnvVar{Name: "xpack.license.self_generated.type", Value: "trial"},
				corev1.EnvVar{Name: "xpack.security.authc.reserved_realm.enabled", Value: "false"},
				corev1.EnvVar{Name: "PROBE_USERNAME", Value: "name"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewEnvironmentVars(
				tt.args.p, tt.args.dataVolume, tt.args.logsVolume, tt.args.probeUser, tt.args.extraFilesSecretVolume,
			)
			for _, v := range tt.wantEnvSubset {
				assert.Contains(t, got, v)
			}
		})
	}
}
