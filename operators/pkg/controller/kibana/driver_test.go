// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func expectedDeploymentParams() *DeploymentParams {
	false := false
	return &DeploymentParams{
		Name:      "test-kb",
		Namespace: "default",
		Selector:  map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "test"},
		Labels:    map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "test"},
		Replicas:  1,
		PodTemplateSpec: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"common.k8s.elastic.co/type":            "kibana",
					"kibana.k8s.elastic.co/name":            "test",
					"kibana.k8s.elastic.co/config-checksum": "c530a02188193a560326ce91e34fc62dcbd5722b45534a3f60957663",
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
						Name: "elasticsearch-certs",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "es-ca-secret",
								Optional:   &false,
							},
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
						Name: http.HTTPCertificatesSecretVolumeName,
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
							Name:      "elasticsearch-certs",
							ReadOnly:  true,
							MountPath: "/usr/share/kibana/config/elasticsearch-certs",
						},
						{
							Name:      "config",
							ReadOnly:  true,
							MountPath: "/usr/share/kibana/config",
						},
						{
							Name:      http.HTTPCertificatesSecretVolumeName,
							ReadOnly:  true,
							MountPath: http.HTTPCertificatesSecretVolumeMountPath,
						},
					},
					Image: "my-image",
					Name:  kbtype.KibanaContainerName,
					Ports: []corev1.ContainerPort{
						{Name: "http", ContainerPort: int32(5601), Protocol: corev1.ProtocolTCP},
					},
					ReadinessProbe: &corev1.Probe{
						FailureThreshold:    3,
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						TimeoutSeconds:      5,
						Handler: corev1.Handler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.FromInt(5601),
								Path:   "/login",
								Scheme: corev1.URISchemeHTTPS,
							},
						},
					},
				}},
				AutomountServiceAccountToken: &false,
			},
		},
	}
}

func Test_driver_deploymentParams(t *testing.T) {
	s := scheme.Scheme
	if err := kbtype.SchemeBuilder.AddToScheme(s); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}

	caSecret := v1alpha1.SecretRef{SecretName: "es-ca-secret"}
	kibanaFixture := kbtype.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: kbtype.KibanaSpec{
			Version:   "7.0.0",
			Image:     "my-image",
			NodeCount: 1,
			Elasticsearch: kbtype.BackendElasticsearch{
				URL: "https://localhost:9200",
				Auth: v1alpha1.ElasticsearchAuth{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-auth",
						},
						Key: "kibana-user",
					},
				},
				CertificateAuthorities: caSecret,
			},
		},
	}

	// add custom labels and resource limits that should be propagated to pods
	kibanaFixtureWithPodTemplate := kibanaFixture
	customResourceLimits := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
	}
	kibanaFixtureWithPodTemplate.Spec.PodTemplate = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"mylabel": "value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      kbtype.KibanaContainerName,
					Resources: customResourceLimits,
				},
			},
		},
	}

	var defaultInitialObjs = []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      caSecret.SecretName,
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

	type args struct {
		kb             *kbtype.Kibana
		initialObjects []runtime.Object
	}

	tests := []struct {
		name    string
		args    args
		want    *DeploymentParams
		wantErr bool
	}{
		{
			name: "without remote objects",
			args: args{
				kb:             &kibanaFixture,
				initialObjects: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "with required remote objects",
			args: args{
				kb:             &kibanaFixture,
				initialObjects: defaultInitialObjs,
			},
			want:    expectedDeploymentParams(),
			wantErr: false,
		},
		{
			name: "with TLS disabled",
			args: args{
				kb: func() *kbtype.Kibana {
					kb := kibanaFixture
					kb.Spec.HTTP.TLS.SelfSignedCertificate = &v1alpha1.SelfSignedCertificate{
						Disabled: true,
					}
					return &kb
				}(),
				initialObjects: defaultInitialObjs,
			},
			want: func() *DeploymentParams {
				params := expectedDeploymentParams()
				params.PodTemplateSpec.Spec.Volumes = params.PodTemplateSpec.Spec.Volumes[:3]
				params.PodTemplateSpec.Spec.Containers[0].VolumeMounts = params.PodTemplateSpec.Spec.Containers[0].VolumeMounts[:3]
				params.PodTemplateSpec.Spec.Containers[0].ReadinessProbe.Handler.HTTPGet.Scheme = corev1.URISchemeHTTP
				return params
			}(),
			wantErr: false,
		},
		{
			name: "with podTemplate specified",
			args: args{
				kb:             &kibanaFixtureWithPodTemplate,
				initialObjects: defaultInitialObjs,
			},
			want: func() *DeploymentParams {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels["mylabel"] = "value"
				for i, c := range p.PodTemplateSpec.Spec.Containers {
					if c.Name == kbtype.KibanaContainerName {
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
				kb: &kibanaFixture,
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      caSecret.SecretName,
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
				},
			},
			want: func() *DeploymentParams {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels = map[string]string{
					"common.k8s.elastic.co/type":            "kibana",
					"kibana.k8s.elastic.co/name":            "test",
					"kibana.k8s.elastic.co/config-checksum": "c5496152d789682387b90ea9b94efcd82a2c6f572f40c016fb86c0d7",
				}
				return p
			}(),
			wantErr: false,
		},
		{
			name: "6.x is supported",
			args: args{
				kb: func() *kbtype.Kibana {
					kb := kibanaFixture
					kb.Spec.Version = "6.5.0"
					return &kb
				}(),
				initialObjects: defaultInitialObjs,
			},
			want: func() *DeploymentParams {
				p := expectedDeploymentParams()
				return p
			}(),
			wantErr: false,
		},
		{
			name: "6.6 docker container already defaults elasticsearch.hosts",
			args: args{
				kb: func() *kbtype.Kibana {
					kb := kibanaFixture
					kb.Spec.Version = "6.6.0"
					return &kb
				}(),
				initialObjects: defaultInitialObjs,
			},
			want:    expectedDeploymentParams(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
			w := watches.NewDynamicWatches()
			err := w.Secrets.InjectScheme(scheme.Scheme)
			assert.NoError(t, err)
			kbVersion, err := version.Parse(tt.args.kb.Spec.Version)
			assert.NoError(t, err)
			d, err := newDriver(client, s, *kbVersion, w, record.NewFakeRecorder(100))
			assert.NoError(t, err)

			got, err := d.deploymentParams(tt.args.kb)
			if (err != nil) != tt.wantErr {
				t.Errorf("driver.deploymentParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// deployment params is deeper than the default 10 levels even though I count only 8
			deep.MaxDepth = 15
			if diff := deep.Equal(got, tt.want); diff != nil {
				t.Error(diff)
			}

		})
	}
}
