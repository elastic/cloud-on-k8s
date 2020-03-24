// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/pod"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var customResourceLimits = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
}

type failingClient struct {
	k8s.Client
}

func (f *failingClient) List(list runtime.Object, opts ...client.ListOption) error {
	return errors.New("client error")
}

func Test_getStrategyType(t *testing.T) {
	// creates `count` of pods belonging to `kbName` Kibana and to `rs-kbName-version` ReplicaSet
	getPods := func(kbName string, podCount int, version string) []runtime.Object {
		var result []runtime.Object
		for i := 0; i < podCount; i++ {
			result = append(result, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Name: fmt.Sprintf("rs-%v-%v", kbName, version)},
					},
					Name:      fmt.Sprintf("pod-%v-%v-%v", kbName, version, i),
					Namespace: "default",
					Labels: map[string]string{
						label.KibanaNameLabelName:    kbName,
						label.KibanaVersionLabelName: version,
					},
				},
			})
		}
		return result
	}

	clearVersionLabels := func(objects []runtime.Object) []runtime.Object {
		for _, object := range objects {
			pod, ok := object.(*corev1.Pod)
			if !ok {
				t.FailNow()
			}

			delete(pod.Labels, label.KibanaVersionLabelName)
		}

		return objects
	}

	tests := []struct {
		name            string
		expectedKbName  string
		expectedVersion string
		initialObjects  []runtime.Object
		clientError     bool
		wantErr         bool
		wantStrategy    appsv1.DeploymentStrategyType
	}{
		{
			name:            "Pods not created yet",
			expectedVersion: "7.4.0",
			expectedKbName:  "test",
			initialObjects:  []runtime.Object{},
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RollingUpdateDeploymentStrategyType,
		},
		{
			name:            "Versions match",
			expectedVersion: "7.4.0",
			expectedKbName:  "test",
			initialObjects:  getPods("test", 3, "7.4.0"),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RollingUpdateDeploymentStrategyType,
		},
		{
			name:            "Versions match - multiple kibana deployments",
			expectedVersion: "7.5.0",
			expectedKbName:  "test2",
			initialObjects:  append(getPods("test", 3, "7.4.0"), getPods("test2", 3, "7.5.0")...),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RollingUpdateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - single kibana deployment",
			expectedVersion: "7.5.0",
			expectedKbName:  "test",
			initialObjects:  getPods("test", 3, "7.4.0"),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - pods partially behind",
			expectedVersion: "7.5.0",
			expectedKbName:  "test",
			initialObjects:  append(getPods("test", 2, "7.5.0"), getPods("test", 1, "7.4.0")...),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - multiple kibana deployments",
			expectedVersion: "7.5.0",
			expectedKbName:  "test2",
			initialObjects:  append(getPods("test", 3, "7.5.0"), getPods("test2", 3, "7.4.0")...),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - multiple versions in flight",
			expectedVersion: "7.5.0",
			expectedKbName:  "test",
			initialObjects: append(
				getPods("test", 1, "7.5.0"),
				append(
					getPods("test", 1, "7.4.0"),
					getPods("test", 1, "7.3.0")...)...),
			clientError:  false,
			wantErr:      false,
			wantStrategy: appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version label missing (operator upgrade case), should assume spec changed",
			expectedVersion: "7.5.0",
			expectedKbName:  "test",
			initialObjects:  clearVersionLabels(getPods("test", 3, "7.5.0")),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Client error",
			expectedVersion: "7.4.0",
			expectedKbName:  "test",
			initialObjects:  getPods("test", 2, "7.4.0"),
			clientError:     true,
			wantErr:         true,
			wantStrategy:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := watches.NewDynamicWatches()

			kb := kibanaFixture()
			kb.Name = tt.expectedKbName
			kb.Spec.Version = tt.expectedVersion

			client := k8s.WrappedFakeClient(tt.initialObjects...)
			if tt.clientError {
				client = &failingClient{}
			}

			d, err := newDriver(client, w, record.NewFakeRecorder(100), kb)
			assert.NoError(t, err)

			strategy, err := d.getStrategyType(kb)
			if tt.wantErr {
				assert.Empty(t, strategy)
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.wantStrategy, strategy)
			}
		})
	}
}

func TestDriverDeploymentParams(t *testing.T) {
	type args struct {
		kb             func() *kbv1.Kibana
		initialObjects func() []runtime.Object
	}

	tests := []struct {
		name    string
		args    args
		want    deployment.Params
		wantErr bool
	}{
		{
			name: "without remote objects",
			args: args{
				kb:             kibanaFixture,
				initialObjects: func() []runtime.Object { return nil },
			},
			want:    deployment.Params{},
			wantErr: true,
		},
		{
			name: "with required remote objects",
			args: args{
				kb:             kibanaFixture,
				initialObjects: defaultInitialObjects,
			},
			want:    expectedDeploymentParams(),
			wantErr: false,
		},
		{
			name: "with TLS disabled",
			args: args{
				kb: func() *kbv1.Kibana {
					kb := kibanaFixture()
					kb.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{
						Disabled: true,
					}
					return kb
				},
				initialObjects: defaultInitialObjects,
			},
			want: func() deployment.Params {
				params := expectedDeploymentParams()
				params.PodTemplateSpec.Spec.Volumes = params.PodTemplateSpec.Spec.Volumes[:3]
				params.PodTemplateSpec.Spec.Containers[0].VolumeMounts = params.PodTemplateSpec.Spec.Containers[0].VolumeMounts[:3]
				params.PodTemplateSpec.Spec.Containers[0].ReadinessProbe.Handler.Exec.Command[2] = `curl -o /dev/null -w "%{http_code}" HTTP://127.0.0.1:5601/login -k -s`
				params.PodTemplateSpec.Spec.Containers[0].Ports[0].Name = "http"
				return params
			}(),
			wantErr: false,
		},
		{
			name: "with podTemplate specified",
			args: args{
				kb:             kibanaFixtureWithPodTemplate,
				initialObjects: defaultInitialObjects,
			},
			want: func() deployment.Params {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels["mylabel"] = "value"
				for i, c := range p.PodTemplateSpec.Spec.Containers {
					if c.Name == kbv1.KibanaContainerName {
						p.PodTemplateSpec.Spec.Containers[i].Resources = customResourceLimits
					}
				}
				return p
			}(),
			wantErr: false,
		},
		{
			name: "Checksum takes secret contents into account",
			args: args{
				kb: kibanaFixture,
				initialObjects: func() []runtime.Object {
					return []runtime.Object{
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "es-ca-secret",
								Namespace: "default",
							},
							Data: map[string][]byte{
								certificates.CertFileName: nil,
							},
						},
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-auth",
								Namespace: "default",
							},
							Data: map[string][]byte{
								"kibana-user": []byte("some-secret"),
							},
						},
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-kb-config",
								Namespace: "default",
							},
							Data: map[string][]byte{
								"kibana.yml": []byte("server.name: test"),
							},
						},
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-kb-http-certs-internal",
								Namespace: "default",
							},
							Data: map[string][]byte{
								"tls.crt": []byte("this is also relevant"),
							},
						},
					}
				},
			},
			want: func() deployment.Params {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels["kibana.k8s.elastic.co/config-checksum"] = "c5496152d789682387b90ea9b94efcd82a2c6f572f40c016fb86c0d7"
				return p
			}(),
			wantErr: false,
		},
		{
			name: "6.8.x is supported",
			args: args{
				kb: func() *kbv1.Kibana {
					kb := kibanaFixture()
					kb.Spec.Version = "6.8.0"
					return kb
				},
				initialObjects: defaultInitialObjects,
			},
			want: func() deployment.Params {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels["kibana.k8s.elastic.co/version"] = "6.8.0"
				return p
			}(),
			wantErr: false,
		},
		{
			name: "6.8 docker container already defaults elasticsearch.hosts",
			args: args{
				kb: func() *kbv1.Kibana {
					kb := kibanaFixture()
					kb.Spec.Version = "6.8.0"
					return kb
				},
				initialObjects: defaultInitialObjects,
			},
			want: func() deployment.Params {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels["kibana.k8s.elastic.co/version"] = "6.8.0"
				return p
			}(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := tt.args.kb()
			initialObjects := tt.args.initialObjects()

			client := k8s.WrappedFakeClient(initialObjects...)
			w := watches.NewDynamicWatches()

			d, err := newDriver(client, w, record.NewFakeRecorder(100), kb)
			require.NoError(t, err)

			got, err := d.deploymentParams(kb)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			if diff := deep.Equal(got, tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestMinSupportedVersion(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "6.7.0 should be unsupported",
			version: "6.6.0",
			wantErr: true,
		},
		{
			name:    "6.8.0 should be supported",
			version: "6.8.0",
			wantErr: false,
		},
		{
			name:    "7.6.0 should be supported",
			version: "7.6.0",
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kb := kibanaFixture()
			kb.Spec.Version = tc.version
			client := k8s.WrappedFakeClient(defaultInitialObjects()...)
			w := watches.NewDynamicWatches()

			_, err := newDriver(client, w, record.NewFakeRecorder(100), kb)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func expectedDeploymentParams() deployment.Params {
	false := false
	return deployment.Params{
		Name:      "test-kb",
		Namespace: "default",
		Selector:  map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "test"},
		Labels:    map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "test"},
		Replicas:  1,
		Strategy:  appsv1.RollingUpdateDeploymentStrategyType,
		PodTemplateSpec: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"common.k8s.elastic.co/type":            "kibana",
					"kibana.k8s.elastic.co/name":            "test",
					"kibana.k8s.elastic.co/config-checksum": "c530a02188193a560326ce91e34fc62dcbd5722b45534a3f60957663",
					"kibana.k8s.elastic.co/version":         "7.0.0",
				},
				Annotations: map[string]string{
					"co.elastic.logs/module": "kibana",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: volume.DataVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "test-kb-config",
								Optional:   &false,
							},
						},
					},
					{
						Name: "elasticsearch-certs",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "es-ca-secret",
								Optional:   &false,
							},
						},
					},
					{
						Name: certificates.HTTPCertificatesSecretVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "test-kb-http-certs-internal",
								Optional:   &false,
							},
						},
					},
				},
				Containers: []corev1.Container{{
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      volume.DataVolumeName,
							ReadOnly:  false,
							MountPath: volume.DataVolumeMountPath,
						},
						{
							Name:      "config",
							ReadOnly:  true,
							MountPath: "/usr/share/kibana/config",
						},
						{
							Name:      "elasticsearch-certs",
							ReadOnly:  true,
							MountPath: "/usr/share/kibana/config/elasticsearch-certs",
						},
						{
							Name:      certificates.HTTPCertificatesSecretVolumeName,
							ReadOnly:  true,
							MountPath: certificates.HTTPCertificatesSecretVolumeMountPath,
						},
					},
					Image: "my-image",
					Name:  kbv1.KibanaContainerName,
					Ports: []corev1.ContainerPort{
						{Name: "https", ContainerPort: int32(5601), Protocol: corev1.ProtocolTCP},
					},
					ReadinessProbe: &corev1.Probe{
						FailureThreshold:    3,
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						TimeoutSeconds:      5,
						Handler: corev1.Handler{
							Exec: &corev1.ExecAction{
								Command: []string{"bash", "-c",
									`curl -o /dev/null -w "%{http_code}" HTTPS://127.0.0.1:5601/login -k -s`,
								},
							},
						},
					},
					Resources: pod.DefaultResources,
				}},
				AutomountServiceAccountToken: &false,
			},
		},
	}
}

func kibanaFixture() *kbv1.Kibana {
	kbFixture := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: kbv1.KibanaSpec{
			Version: "7.0.0",
			Image:   "my-image",
			Count:   1,
		},
	}

	kbFixture.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "test-auth",
		AuthSecretKey:  "kibana-user",
		CASecretName:   "es-ca-secret",
		URL:            "https://localhost:9200",
	})

	return kbFixture
}

func kibanaFixtureWithPodTemplate() *kbv1.Kibana {
	kbFixture := kibanaFixture()
	kbFixture.Spec.PodTemplate = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"mylabel": "value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      kbv1.KibanaContainerName,
					Resources: customResourceLimits,
				},
			},
		},
	}

	return kbFixture
}

func defaultInitialObjects() []runtime.Object {
	return []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "es-ca-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				certificates.CertFileName: nil,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-auth",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"kibana-user": nil,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kb-config",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"kibana.yml": []byte("server.name: test"),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-kb-http-certs-internal",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"tls.crt": nil,
			},
		},
	}
}
