// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func expectedDeploymentParams() *DeploymentParams {
	false := false
	return &DeploymentParams{
		Name:      "kb-kibana",
		Namespace: "default",
		Selector:  map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "kb"},
		Labels:    map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "kb"},
		PodLabels: map[string]string{
			"common.k8s.elastic.co/type":            "kibana",
			"kibana.k8s.elastic.co/name":            "kb",
			"kibana.k8s.elastic.co/config-checksum": "d14a028c2a3a2bc9476102bb288234c415a2b01f828ea62ac5b3e42f",
		},
		Replicas: 1,
		PodSpec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "elasticsearch-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "es-ca-secret",
							Optional:   &false,
						},
					},
				},
			},
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "elasticsearch-certs",
						ReadOnly:  true,
						MountPath: "/usr/share/kibana/config/elasticsearch-certs",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "ELASTICSEARCH_HOSTS",
						Value: "https://localhost:9200",
					},
					{
						Name:  "ELASTICSEARCH_USERNAME",
						Value: "kibana-user",
					},
					{
						Name: "ELASTICSEARCH_PASSWORD",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "kb-auth",
								},
								Key: "kibana-user",
							},
						},
					},
					{
						Name:  "ELASTICSEARCH_SSL_CERTIFICATEAUTHORITIES",
						Value: "/usr/share/kibana/config/elasticsearch-certs/ca.pem",
					},
					{
						Name:  "ELASTICSEARCH_SSL_VERIFICATIONMODE",
						Value: "certificate",
					},
				},
				Image: "my-image",
				Name:  "kibana",
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
							Path:   "/",
							Scheme: corev1.URISchemeHTTP,
						},
					},
				},
			}},
			AutomountServiceAccountToken: &false,
		},
	}

}

func Test_driver_deploymentParams(t *testing.T) {
	s := scheme.Scheme
	if err := kbtype.SchemeBuilder.AddToScheme(s); err != nil {
		assert.Fail(t, "failed to build custom scheme")
	}

	caSecret := "es-ca-secret"
	kibanaFixture := kbtype.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb",
			Namespace: "default",
		},
		Spec: kbtype.KibanaSpec{
			Version:   "7.0.0",
			Image:     "my-image",
			NodeCount: 1,
			Elasticsearch: kbtype.BackendElasticsearch{
				URL: "https://localhost:9200",
				Auth: kbtype.ElasticsearchAuth{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "kb-auth",
						},
						Key: "kibana-user",
					},
				},
				CaCertSecret: &caSecret,
			},
			Expose: "LoadBalancer",
		},
	}

	var defaultInitialObjs = []runtime.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      caSecret,
				Namespace: "default",
			},
			Data: map[string][]byte{
				"ca.pem": nil,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kb-auth",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"kibana-user": nil,
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
			name: "Checksum takes secret contents into account",
			args: args{
				kb: &kibanaFixture,
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      caSecret,
							Namespace: "default",
						},
						Data: map[string][]byte{
							"ca.pem": nil,
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kb-auth",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"kibana-user": []byte("some-secret"),
						},
					},
				},
			},
			want: func() *DeploymentParams {
				p := expectedDeploymentParams()
				p.PodLabels = map[string]string{
					"common.k8s.elastic.co/type":            "kibana",
					"kibana.k8s.elastic.co/name":            "kb",
					"kibana.k8s.elastic.co/config-checksum": "5d26adcdb6e5e6be930802dc6639233ece8c2a3bc2cf8b8dffa69602",
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
					kb.Spec.Version = "6.6.0"
					return &kb
				}(),
				initialObjects: defaultInitialObjs,
			},
			want: func() *DeploymentParams {
				p := expectedDeploymentParams()
				p.PodSpec.Containers[0].Env[0].Name = "ELASTICSEARCH_URL"
				return p
			}(),
			wantErr: false,
		},
		{
			name: "6.7 docker container already defaults elasticsearch.hosts",
			args: args{
				kb: func() *kbtype.Kibana {
					kb := kibanaFixture
					kb.Spec.Version = "6.7.0"
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
			version, err := version.Parse(tt.args.kb.Spec.Version)
			assert.NoError(t, err)
			d, err := newDriver(client, s, *version, w)
			assert.NoError(t, err)
			got, err := d.deploymentParams(tt.args.kb)
			if (err != nil) != tt.wantErr {
				t.Errorf("driver.deploymentParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := deep.Equal(got, tt.want); diff != nil {
				t.Error(diff)
			}

		})
	}
}
