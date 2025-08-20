// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	commonvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/initcontainer"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/network"
	kbvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var customResourceLimits = corev1.ResourceRequirements{
	Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
}

func Test_getStrategyType(t *testing.T) {
	// creates `count` of pods belonging to `kbName` Kibana and to `rs-kbName-version` ReplicaSet
	getPods := func(kbName string, podCount int, version string) []client.Object {
		var result []client.Object
		for i := 0; i < podCount; i++ {
			result = append(result, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Name: fmt.Sprintf("rs-%v-%v", kbName, version)},
					},
					Name:      fmt.Sprintf("pod-%v-%v-%v", kbName, version, i),
					Namespace: "default",
					Labels: map[string]string{
						kblabel.KibanaNameLabelName:    kbName,
						kblabel.KibanaVersionLabelName: version,
					},
				},
			})
		}
		return result
	}

	clearVersionLabels := func(objects []client.Object) []client.Object {
		for _, object := range objects {
			pod, ok := object.(*corev1.Pod)
			if !ok {
				t.FailNow()
			}

			delete(pod.Labels, kblabel.KibanaVersionLabelName)
		}

		return objects
	}

	tests := []struct {
		name            string
		expectedKbName  string
		expectedVersion string
		initialObjects  []client.Object
		clientError     bool
		wantErr         bool
		wantStrategy    appsv1.DeploymentStrategyType
	}{
		{
			name:            "Pods not created yet",
			expectedVersion: "7.17.0",
			expectedKbName:  "test",
			initialObjects:  []client.Object{},
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RollingUpdateDeploymentStrategyType,
		},
		{
			name:            "Versions match",
			expectedVersion: "7.17.0",
			expectedKbName:  "test",
			initialObjects:  getPods("test", 3, "7.17.0"),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RollingUpdateDeploymentStrategyType,
		},
		{
			name:            "Versions match - multiple kibana deployments",
			expectedVersion: "8.5.0",
			expectedKbName:  "test2",
			initialObjects:  append(getPods("test", 3, "8.4.0"), getPods("test2", 3, "8.5.0")...),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RollingUpdateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - single kibana deployment",
			expectedVersion: "8.5.0",
			expectedKbName:  "test",
			initialObjects:  getPods("test", 3, "8.4.0"),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - pods partially behind",
			expectedVersion: "8.5.0",
			expectedKbName:  "test",
			initialObjects:  append(getPods("test", 2, "8.5.0"), getPods("test", 1, "8.4.0")...),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - multiple kibana deployments",
			expectedVersion: "8.5.0",
			expectedKbName:  "test2",
			initialObjects:  append(getPods("test", 3, "8.5.0"), getPods("test2", 3, "8.4.0")...),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version mismatch - multiple versions in flight",
			expectedVersion: "8.5.0",
			expectedKbName:  "test",
			initialObjects: append(
				getPods("test", 1, "8.5.0"),
				append(
					getPods("test", 1, "8.4.0"),
					getPods("test", 1, "8.3.0")...)...),
			clientError:  false,
			wantErr:      false,
			wantStrategy: appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Version label missing (operator upgrade case), should assume spec changed",
			expectedVersion: "8.5.0",
			expectedKbName:  "test",
			initialObjects:  clearVersionLabels(getPods("test", 3, "8.5.0")),
			clientError:     false,
			wantErr:         false,
			wantStrategy:    appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:            "Client error",
			expectedVersion: "8.4.0",
			expectedKbName:  "test",
			initialObjects:  getPods("test", 2, "8.4.0"),
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

			client := k8s.NewFakeClient(tt.initialObjects...)
			if tt.clientError {
				client = k8s.NewFailingClient(errors.New("client error"))
			}

			d, err := newDriver(client, w, record.NewFakeRecorder(100), kb, corev1.IPv4Protocol)
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
		kb                            func() *kbv1.Kibana
		initialObjects                func() []client.Object
		policyAnnotations             map[string]string
		setDefaultSecurityContextFlag bool
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
				initialObjects: func() []client.Object { return nil },
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
			want:    pre710(expectedDeploymentParams()),
			wantErr: false,
		},
		{
			name: "with policy annotations",
			args: args{
				kb:                kibanaFixture,
				initialObjects:    defaultInitialObjects,
				policyAnnotations: map[string]string{"policy.k8s.elastic.co/kibana-config-hash": "2123345"},
			},
			want:    pre710(expectedDeploymentWithPolicyAnnotations(map[string]string{"policy.k8s.elastic.co/kibana-config-hash": "2123345"})),
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
				params := pre710(expectedDeploymentParams())
				params.PodTemplateSpec.Spec.Volumes = params.PodTemplateSpec.Spec.Volumes[1:]
				params.PodTemplateSpec.Spec.InitContainers[0].VolumeMounts = params.PodTemplateSpec.Spec.InitContainers[0].VolumeMounts[1:]
				params.PodTemplateSpec.Spec.Containers[0].VolumeMounts = params.PodTemplateSpec.Spec.Containers[0].VolumeMounts[1:]
				params.PodTemplateSpec.Spec.Containers[0].ReadinessProbe.ProbeHandler.HTTPGet.Scheme = corev1.URISchemeHTTP
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
				p := pre710(expectedDeploymentParams())
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
				initialObjects: func() []client.Object {
					return []client.Object{
						&corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "es-ca-secret",
								Namespace: "default",
							},
							Data: map[string][]byte{
								certificates.CAFileName: nil,
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
				p := pre710(expectedDeploymentParams())
				p.PodTemplateSpec.Annotations["kibana.k8s.elastic.co/config-hash"] = "2368465874"
				return p
			}(),
			wantErr: false,
		},
		{
			name: "7.10+ contains security contexts",
			args: args{
				kb: func() *kbv1.Kibana {
					kb := kibanaFixture()
					kb.Spec.Version = "7.17.0"
					return kb
				},
				initialObjects:                defaultInitialObjects,
				setDefaultSecurityContextFlag: true,
			},
			want: func() deployment.Params {
				p := expectedDeploymentParams()
				p.PodTemplateSpec.Labels["kibana.k8s.elastic.co/version"] = "7.17.0"
				p.PodTemplateSpec.Spec.SecurityContext = &corev1.PodSecurityContext{
					FSGroup: ptr.To[int64](1000),
				}
				return p
			}(),
			wantErr: false,
		},
		{
			name: "7.10+ does not contain default security context when flag is not set",
			args: args{
				kb: func() *kbv1.Kibana {
					kb := kibanaFixture()
					kb.Spec.Version = "7.17.0"
					return kb
				},
				initialObjects:                defaultInitialObjects,
				setDefaultSecurityContextFlag: false,
			},
			want: func() deployment.Params {
				p := pre710(expectedDeploymentParams())
				p.PodTemplateSpec.Labels["kibana.k8s.elastic.co/version"] = "7.17.0"
				return p
			}(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := tt.args.kb()
			initialObjects := tt.args.initialObjects()

			client := k8s.NewFakeClient(initialObjects...)
			w := watches.NewDynamicWatches()

			d, err := newDriver(client, w, record.NewFakeRecorder(100), kb, corev1.IPv4Protocol)
			require.NoError(t, err)

			got, err := d.deploymentParams(context.Background(), kb, tt.args.policyAnnotations, "", tt.args.setDefaultSecurityContextFlag, metadata.Propagate(kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}))
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
			name:    "6.8.0 should be unsupported",
			version: "6.8.0",
			wantErr: true,
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
			client := k8s.NewFakeClient(defaultInitialObjects()...)
			w := watches.NewDynamicWatches()

			_, err := newDriver(client, w, record.NewFakeRecorder(100), kb, corev1.IPv4Protocol)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func expectedDeploymentParams() deployment.Params {
	falseVal := false
	return deployment.Params{
		Name:      "test-kb",
		Namespace: "default",
		Selector:  map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "test"},
		Metadata:  metadata.Metadata{Labels: map[string]string{"common.k8s.elastic.co/type": "kibana", "kibana.k8s.elastic.co/name": "test"}},
		Replicas:  1,
		Strategy:  appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
		PodTemplateSpec: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"common.k8s.elastic.co/type":    "kibana",
					"kibana.k8s.elastic.co/name":    "test",
					"kibana.k8s.elastic.co/version": "7.17.0",
				},
				Annotations: map[string]string{
					"co.elastic.logs/module":            "kibana",
					"kibana.k8s.elastic.co/config-hash": "272660573",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: certificates.HTTPCertificatesSecretVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "test-kb-http-certs-internal",
								Optional:   &falseVal,
							},
						},
					},
					{
						Name: "elastic-internal-kibana-config",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "test-kb-config",
								Optional:   &falseVal,
							},
						},
					},
					{
						Name: initcontainer.ConfigSharedVolume.VolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "elasticsearch-certs",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "es-ca-secret",
								Optional:   &falseVal,
							},
						},
					},
					{
						Name: kbvolume.DataVolumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "kibana-logs",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "kibana-plugins",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
					{
						Name: "kibana-scripts",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "test-kb-scripts",
								},
								DefaultMode: ptr.To(int32(0755)),
								Optional:    ptr.To(false),
							},
						},
					},
					{
						Name: "temp-volume",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
				InitContainers: []corev1.Container{
					{
						Name:            "elastic-internal-init",
						ImagePullPolicy: corev1.PullIfNotPresent,
						Image:           "my-image",
						Command:         []string{"/usr/bin/env", "bash", "-c", "/mnt/elastic-internal/scripts/init.sh"},
						SecurityContext: nil,
						Env: []corev1.EnvVar{
							{Name: settings.EnvPodIP, Value: "", ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
							}},
							{Name: settings.EnvPodName, Value: "", ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
							}},
							{Name: settings.EnvNodeName, Value: "", ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"},
							}},
							{Name: settings.EnvNamespace, Value: "", ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"},
							}},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      certificates.HTTPCertificatesSecretVolumeName,
								ReadOnly:  true,
								MountPath: certificates.HTTPCertificatesSecretVolumeMountPath,
							},
							{
								Name:      "elastic-internal-kibana-config",
								ReadOnly:  true,
								MountPath: "/mnt/elastic-internal/kibana-config",
							},
							{
								Name:      "elastic-internal-kibana-config-local",
								ReadOnly:  false,
								MountPath: "/mnt/elastic-internal/kibana-config-local",
							},
							{
								Name:      "elasticsearch-certs",
								ReadOnly:  true,
								MountPath: "/usr/share/kibana/config/elasticsearch-certs",
							},
							{
								Name:      kbvolume.DataVolumeName,
								ReadOnly:  falseVal,
								MountPath: kbvolume.DataVolumeMountPath,
							},
							{
								Name:      "kibana-logs",
								ReadOnly:  falseVal,
								MountPath: "/usr/share/kibana/logs",
							},
							{
								Name:      "kibana-plugins",
								ReadOnly:  falseVal,
								MountPath: "/mnt/elastic-internal/kibana-plugins-local",
							},
							{
								Name:      "kibana-scripts",
								ReadOnly:  true,
								MountPath: "/mnt/elastic-internal/scripts",
							},
							{
								Name:      "temp-volume",
								ReadOnly:  falseVal,
								MountPath: "/tmp",
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceMemory: resource.MustParse("50Mi"),
								corev1.ResourceCPU:    resource.MustParse("0.1"),
							},
							Limits: map[corev1.ResourceName]resource.Quantity{
								// Memory limit should be at least 12582912 when running with CRI-O
								corev1.ResourceMemory: resource.MustParse("50Mi"),
								corev1.ResourceCPU:    resource.MustParse("0.1"),
							},
						},
					},
				},
				Containers: []corev1.Container{{
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      certificates.HTTPCertificatesSecretVolumeName,
							ReadOnly:  true,
							MountPath: certificates.HTTPCertificatesSecretVolumeMountPath,
						},
						{
							Name:      "elastic-internal-kibana-config",
							ReadOnly:  true,
							MountPath: kbvolume.InternalConfigVolumeMountPath,
						},
						initcontainer.ConfigSharedVolume.VolumeMount(),
						{
							Name:      "elasticsearch-certs",
							ReadOnly:  true,
							MountPath: "/usr/share/kibana/config/elasticsearch-certs",
						},
						{
							Name:      kbvolume.DataVolumeName,
							ReadOnly:  falseVal,
							MountPath: kbvolume.DataVolumeMountPath,
						},
						{
							Name:      "kibana-logs",
							ReadOnly:  falseVal,
							MountPath: "/usr/share/kibana/logs",
						},
						{
							Name:      "kibana-plugins",
							ReadOnly:  falseVal,
							MountPath: "/usr/share/kibana/plugins",
						},
						{
							Name:      "kibana-scripts",
							ReadOnly:  true,
							MountPath: "/mnt/elastic-internal/scripts",
						},
						{
							Name:      "temp-volume",
							ReadOnly:  falseVal,
							MountPath: "/tmp",
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
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port:   intstr.FromInt(5601),
								Path:   "/login",
								Scheme: corev1.URISchemeHTTPS,
							},
						},
					},
					Resources:       DefaultResources,
					SecurityContext: &defaultSecurityContext,
				}},
				AutomountServiceAccountToken: &falseVal,
			},
		},
	}
}

func expectedDeploymentWithPolicyAnnotations(policyAnnotations map[string]string) deployment.Params {
	deploymentParams := expectedDeploymentParams()

	for k, v := range policyAnnotations {
		deploymentParams.PodTemplateSpec.Annotations[k] = v
	}
	return deploymentParams
}

func pre710(params deployment.Params) deployment.Params {
	params.PodTemplateSpec.Spec.Containers[0].SecurityContext = nil
	params.PodTemplateSpec.Spec.InitContainers[0].SecurityContext = nil
	params.PodTemplateSpec.Spec.SecurityContext = nil
	params.PodTemplateSpec.Spec.Volumes = append(params.PodTemplateSpec.Spec.Volumes[:5], params.PodTemplateSpec.Spec.Volumes[6], params.PodTemplateSpec.Spec.Volumes[7])
	params.PodTemplateSpec.Spec.InitContainers[0].VolumeMounts = append(params.PodTemplateSpec.Spec.InitContainers[0].VolumeMounts[:5], params.PodTemplateSpec.Spec.InitContainers[0].VolumeMounts[6], params.PodTemplateSpec.Spec.InitContainers[0].VolumeMounts[7])
	params.PodTemplateSpec.Spec.Containers[0].VolumeMounts = append(params.PodTemplateSpec.Spec.Containers[0].VolumeMounts[:5], params.PodTemplateSpec.Spec.Containers[0].VolumeMounts[6], params.PodTemplateSpec.Spec.Containers[0].VolumeMounts[7])
	return params
}

func kibanaFixture() *kbv1.Kibana {
	kbFixture := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: kbv1.KibanaSpec{
			Version: "7.17.0",
			Image:   "my-image",
			Count:   1,
			ElasticsearchRef: commonv1.ObjectSelector{
				Name: "es",
			},
		},
	}

	kbFixture.EsAssociation().SetAssociationConf(&commonv1.AssociationConf{
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

func defaultInitialObjects() []client.Object {
	return []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "es-ca-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				certificates.CAFileName: nil,
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
				"ca.crt": nil,
			},
		},
	}
}

func TestNewService(t *testing.T) {
	testCases := []struct {
		name     string
		httpConf commonv1.HTTPConfig
		wantSvc  func() corev1.Service
	}{
		{
			name: "no TLS",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
			wantSvc: mkService,
		},
		{
			name: "self-signed certificate",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
							{
								DNS: "kibana-test.local",
							},
						},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "user-provided certificate",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					Certificate: commonv1.SecretRef{
						SecretName: "my-cert",
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "service template",
			httpConf: commonv1.HTTPConfig{
				Service: commonv1.ServiceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"foo": "bar"},
						Annotations: map[string]string{"bar": "baz"},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Labels["foo"] = "bar"
				svc.Annotations = map[string]string{"bar": "baz"}
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kb := kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kibana-test",
					Namespace: "test",
				},
				Spec: kbv1.KibanaSpec{
					HTTP: tc.httpConf,
				},
			}
			haveSvc := NewService(kb, metadata.Propagate(&kb, metadata.Metadata{Labels: kb.GetIdentityLabels()}))
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kibana-test-kb-http",
			Namespace: "test",
			Labels: map[string]string{
				kblabel.KibanaNameLabelName: "kibana-test",
				commonv1.TypeLabelName:      kblabel.Type,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     network.HTTPPort,
				},
			},
			Selector: map[string]string{
				kblabel.KibanaNameLabelName: "kibana-test",
				commonv1.TypeLabelName:      kblabel.Type,
			},
		},
	}
}

func TestDriver_buildVolumes(t *testing.T) {
	tests := []struct {
		name       string
		kb         *kbv1.Kibana
		assertions func(t *testing.T, volumes []commonvolume.VolumeLike, err error)
	}{
		{
			name: "without associations",
			kb: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test-ns",
				},
				Spec: kbv1.KibanaSpec{
					Version: "7.10.0",
				},
			},
			assertions: func(t *testing.T, volumes []commonvolume.VolumeLike, err error) {
				t.Helper()
				require.NoError(t, err)
				// Should have default volumes but no association certificate volumes
				assert.Equal(t, 4, len(volumes)) // DataVolume, ConfigSharedVolume, ConfigVolume, HTTPCertsVolume
			},
		},
		{
			name: "with EPR association and CA configured",
			kb: func() *kbv1.Kibana {
				kb := &kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"association.k8s.elastic.co/epr-conf": `{"authSecretName":"-","authSecretKey":"","caCertProvided":true,"caSecretName":"test-epr-ca","url":"https://test-epr:8080"}`,
						},
					},
					Spec: kbv1.KibanaSpec{
						Version: "7.10.0",
						PackageRegistryRef: commonv1.ObjectSelector{
							Name: "test-epr",
						},
					},
				}
				return kb
			}(),
			assertions: func(t *testing.T, volumes []commonvolume.VolumeLike, err error) {
				t.Helper()
				require.NoError(t, err)
				// Should have default volumes + EPR certificate volume
				assert.Equal(t, 5, len(volumes)) // DataVolume, ConfigSharedVolume, ConfigVolume, HTTPCertsVolume, EPRCertsVolume

				// Check that EPR certificate volume is present
				foundEPRCertsVolume := false
				for _, vol := range volumes {
					if vol.Name() == "epr-certs" {
						foundEPRCertsVolume = true
						assert.Equal(t, eprCertsVolumeMountPath, vol.VolumeMount().MountPath)
						break
					}
				}
				assert.True(t, foundEPRCertsVolume, "EPR certificates volume should be present")
			},
		},
		{
			name: "with multiple associations",
			kb: func() *kbv1.Kibana {
				kb := &kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"association.k8s.elastic.co/es-conf":  `{"authSecretName":"test-es-user","authSecretKey":"token","caCertProvided":true,"caSecretName":"test-es-ca","url":"https://test-es:9200"}`,
							"association.k8s.elastic.co/epr-conf": `{"authSecretName":"-","authSecretKey":"","caCertProvided":true,"caSecretName":"test-epr-ca","url":"https://test-epr:8080"}`,
						},
					},
					Spec: kbv1.KibanaSpec{
						Version: "7.10.0",
						ElasticsearchRef: commonv1.ObjectSelector{
							Name: "test-es",
						},
						PackageRegistryRef: commonv1.ObjectSelector{
							Name: "test-epr",
						},
					},
				}
				return kb
			}(),
			assertions: func(t *testing.T, volumes []commonvolume.VolumeLike, err error) {
				t.Helper()
				require.NoError(t, err)
				// Should have default volumes + ES certificate volume + EPR certificate volume
				assert.Equal(t, 6, len(volumes)) // DataVolume, ConfigSharedVolume, ConfigVolume, HTTPCertsVolume, ESCertsVolume, EPRCertsVolume

				// Check that both certificate volumes are present
				foundESCertsVolume := false
				foundEPRCertsVolume := false
				for _, vol := range volumes {
					if vol.Name() == "elasticsearch-certs" {
						foundESCertsVolume = true
						assert.Equal(t, esCertsVolumeMountPath, vol.VolumeMount().MountPath)
					} else if vol.Name() == "epr-certs" {
						foundEPRCertsVolume = true
						assert.Equal(t, eprCertsVolumeMountPath, vol.VolumeMount().MountPath)
					}
				}
				assert.True(t, foundESCertsVolume, "ES certificates volume should be present")
				assert.True(t, foundEPRCertsVolume, "EPR certificates volume should be present")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &driver{}
			volumes, err := d.buildVolumes(tt.kb)
			tt.assertions(t, volumes, err)
		})
	}
}
