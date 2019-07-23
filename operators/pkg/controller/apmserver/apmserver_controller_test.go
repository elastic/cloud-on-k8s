// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"testing"

	apmv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var certSecretName = "test-apm-server-apm-http-certs-internal" // nolint

type testParams = DeploymentParams

func (tp testParams) withConfigChecksum(checksum string) testParams {
	tp.PodTemplateSpec.Labels["apm.k8s.elastic.co/config-file-checksum"] = checksum
	return tp
}

func (tp testParams) withVolume(i int, volume corev1.Volume) testParams {
	tp.PodTemplateSpec.Spec.Volumes = append(tp.PodTemplateSpec.Spec.Volumes[:i], append([]corev1.Volume{volume}, tp.PodTemplateSpec.Spec.Volumes[i:]...)...)
	return tp
}

func (tp testParams) withVolumeMount(i int, mnt corev1.VolumeMount) testParams {
	for j := range tp.PodTemplateSpec.Spec.Containers {
		mounts := tp.PodTemplateSpec.Spec.Containers[j].VolumeMounts
		mounts = append(mounts[:i], append([]corev1.VolumeMount{mnt}, mounts[i:]...)...)
		tp.PodTemplateSpec.Spec.Containers[i].VolumeMounts = mounts
	}
	return tp
}

func (tp testParams) withInitContainer() testParams {
	tp.PodTemplateSpec.Spec.InitContainers = []corev1.Container{
		{
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "apmserver-data",
					MountPath: DataVolumePath,
				},
				{
					Name:      "config",
					ReadOnly:  true,
					MountPath: "/usr/share/apm-server/config/config-secret",
				},
				{
					Name:      "config-volume",
					ReadOnly:  false,
					MountPath: "/usr/share/apm-server/config",
				},
			},
			Name:  "",
			Image: "docker.elastic.co/apm/apm-server:1.0",
			Env: []corev1.EnvVar{{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			},
				{
					Name: "POD_IP",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							APIVersion: "v1",
							FieldPath:  "status.podIP",
						},
					},
				},
			},
		},
	}
	return tp
}

func expectedDeploymentParams() testParams {
	false := false
	return DeploymentParams{
		Name:      "test-apm-server-apm-server",
		Namespace: "",
		Selector:  map[string]string{"apm.k8s.elastic.co/name": "test-apm-server", "common.k8s.elastic.co/type": "apm-server"},
		Labels:    map[string]string{"apm.k8s.elastic.co/name": "test-apm-server", "common.k8s.elastic.co/type": "apm-server"},
		PodTemplateSpec: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"common.k8s.elastic.co/type":              "apm-server",
					"apm.k8s.elastic.co/name":                 "test-apm-server",
					"apm.k8s.elastic.co/config-file-checksum": "d14a028c2a3a2bc9476102bb288234c415a2b01f828ea62ac5b3e42f",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "test-apm-config",
								Optional:   &false,
							},
						},
					},
					{
						Name: "config-volume",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: http.HTTPCertificatesSecretVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: certSecretName,
								Optional:   &false,
							},
						},
					},
				},
				Containers: []corev1.Container{{
					VolumeMounts: []corev1.VolumeMount{

						{
							Name:      "config",
							ReadOnly:  true,
							MountPath: "/usr/share/apm-server/config/config-secret",
						},
						{
							Name:      "config-volume",
							ReadOnly:  false,
							MountPath: "/usr/share/apm-server/config",
						},
						{
							Name:      "elastic-internal-http-certificates",
							ReadOnly:  true,
							MountPath: "/mnt/elastic-internal/http-certs",
						},
					},
					Name:  apmv1alpha1.APMServerContainerName,
					Image: "docker.elastic.co/apm/apm-server:1.0",
					Command: []string{
						"apm-server",
						"run",
						"-e",
						"-c",
						"config/config-secret/apm-server.yml",
					},
					Env: []corev1.EnvVar{{
						Name: "POD_NAME",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.name",
							},
						},
					},
						{
							Name: "SECRET_TOKEN",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "test-apm-server-apm-token",
									},
									Key: "secret-token",
								},
							},
						},
					},
					Ports: []corev1.ContainerPort{
						{Name: "http", ContainerPort: int32(8200), Protocol: corev1.ProtocolTCP},
					},
					ReadinessProbe: &corev1.Probe{
						FailureThreshold:    3,
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						TimeoutSeconds:      5,
						Handler: corev1.Handler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.FromInt(8200),
								Path:   "/",
								Scheme: corev1.URISchemeHTTPS,
							},
						},
					},
				}},
				AutomountServiceAccountToken: &false,
			},
		},
		Replicas: 0,
	}

}

func TestReconcileApmServer_deploymentParams(t *testing.T) {

	s := scheme.Scheme
	if err := apmv1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Error(err)
	}

	apmFixture := &apmv1alpha1.ApmServer{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-apm-server",
		},
		TypeMeta: v1.TypeMeta{
			Kind: "apmserver",
		},
	}
	defaultPodSpecParams := PodSpecParams{
		Version: "1.0",
		ApmServerSecret: corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-apm-server-apm-token",
			},
		},
		ConfigSecret: corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-apm-config",
			},
		},
		keystoreResources: nil,
	}

	type args struct {
		as             *apmv1alpha1.ApmServer
		podSpecParams  PodSpecParams
		initialObjects []runtime.Object
	}
	tests := []struct {
		name    string
		args    args
		want    testParams
		wantErr bool
	}{
		{
			name: "default params",
			args: args{
				as:            apmFixture,
				podSpecParams: defaultPodSpecParams,
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: certSecretName,
						},
					},
				},
			},
			want:    expectedDeploymentParams(),
			wantErr: false,
		},
		{
			name: "certificate secret influences checksum",
			args: args{
				as:            apmFixture,
				podSpecParams: defaultPodSpecParams,
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: certSecretName,
						},
						Data: map[string][]byte{
							certificates.CertFileName: []byte("bar"),
						},
					},
				},
			},
			want:    expectedDeploymentParams().withConfigChecksum("07daf010de7f7f0d8d76a76eb8d1eb40182c8d1e7a3877a6686c9bf0"),
			wantErr: false,
		},
		{
			name: "config influences checksum",
			args: args{
				as: apmFixture,
				podSpecParams: func() PodSpecParams {
					params := defaultPodSpecParams
					params.ConfigSecret = corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: "test-apm-config",
						},
						Data: map[string][]byte{
							"apm-server.yml": []byte("baz"),
						},
					}
					return params
				}(),
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: certSecretName,
						},
					},
				},
			},
			want:    expectedDeploymentParams().withConfigChecksum("1846d1bd30922b6492a1a28bc940fd00efcd2d9bfb00e34e94bf8048"),
			wantErr: false,
		},
		{
			name: "keystore version influences checksum",
			args: args{
				as: apmFixture,
				podSpecParams: func() PodSpecParams {
					params := defaultPodSpecParams
					params.keystoreResources = &keystore.Resources{
						Volume: corev1.Volume{
							Name: "keystore-volume",
						},
						InitContainer: corev1.Container{},
						Version:       "1",
					}
					return params
				}(),
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: v1.ObjectMeta{
							Name: certSecretName,
						},
					},
				},
			},
			want: expectedDeploymentParams().
				withConfigChecksum("e25388fde8290dc286a6164fa2d97e551b53498dcbf7bc378eb1f178").
				withVolume(0, corev1.Volume{
					Name: "apmserver-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				}).
				withVolume(3, corev1.Volume{
					Name: "keystore-volume",
				}).
				withVolumeMount(0, corev1.VolumeMount{
					Name:      "apmserver-data",
					MountPath: DataVolumePath,
				}).
				withInitContainer(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
			w := watches.NewDynamicWatches()
			require.NoError(t, w.Secrets.InjectScheme(s))
			r := &ReconcileApmServer{
				Client:         client,
				scheme:         s,
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: w,
			}
			got, err := r.deploymentParams(tt.args.as, tt.args.podSpecParams)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileApmServer.deploymentParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			deep.MaxDepth = 15
			if diff := deep.Equal(got, tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}
