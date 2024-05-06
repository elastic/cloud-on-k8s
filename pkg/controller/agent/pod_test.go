// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	fleetCertsFixture = &certificates.CertificatesSecret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fleet-certs-secret-name",
			},
			Data: map[string][]byte{
				"ca.crt":  []byte("a CA cert"),
				"tls.key": []byte("a private key"),
				"tls.crt": []byte("the server cert"),
			},
		},
	}
	wellKnownCACertsFixture = &certificates.CertificatesSecret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "fleet-certs-secret-name",
			},
			Data: map[string][]byte{
				"tls.key": []byte("a private key"),
				"tls.crt": []byte("the server cert"),
			},
		},
	}
)

func Test_amendBuilderForFleetMode(t *testing.T) {
	optional := false

	for _, tt := range []struct {
		name        string
		params      Params
		fleetCerts  *certificates.CertificatesSecret
		wantPodSpec corev1.PodSpec
	}{
		{
			name: "running elastic agent, with fleet server, without es/kb association",
			params: Params{
				Agent: agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "default",
					},
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
					},
				},
				Client: k8s.NewFakeClient(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-agent-http",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "https",
								Port: 8220,
							},
						},
					},
				}),
			},
			fleetCerts: fleetCertsFixture,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = []corev1.Volume{
					{
						Name: "fleet-certs",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "fleet-certs-secret-name",
								Optional:   &optional,
							},
						},
					},
				}

				ps.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "fleet-certs",
						ReadOnly:  true,
						MountPath: "/usr/share/fleet-server/config/http-certs",
					},
				}

				ps.Containers[0].Ports = []corev1.ContainerPort{
					{
						Name:          "https",
						ContainerPort: 8220,
						Protocol:      corev1.ProtocolTCP,
					},
				}

				ps.Containers[0].Env = []corev1.EnvVar{
					{
						Name:  "FLEET_CA",
						Value: "/usr/share/fleet-server/config/http-certs/ca.crt",
					},
					{
						Name:  "FLEET_SERVER_CERT",
						Value: "/usr/share/fleet-server/config/http-certs/tls.crt",
					},
					{
						Name:  "FLEET_SERVER_CERT_KEY",
						Value: "/usr/share/fleet-server/config/http-certs/tls.key",
					},
					{
						Name:  "FLEET_SERVER_ENABLE",
						Value: "true",
					},
					{
						Name:  "FLEET_URL",
						Value: "https://agent-agent-http.default.svc:8220",
					},
					{
						Name:  "CONFIG_PATH",
						Value: "/usr/share/elastic-agent",
					},
				}

				ps.Containers[0].Resources = corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
				}

				return ps
			}),
		},
		{
			name: "running elastic agent, with fleet server, without es/kb association, with well known CA",
			params: Params{
				Agent: agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "default",
					},
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
					},
				},
				Client: k8s.NewFakeClient(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-agent-http",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "https",
								Port: 8220,
							},
						},
					},
				}),
			},
			fleetCerts: wellKnownCACertsFixture,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = []corev1.Volume{
					{
						Name: "fleet-certs",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "fleet-certs-secret-name",
								Optional:   &optional,
							},
						},
					},
				}

				ps.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "fleet-certs",
						ReadOnly:  true,
						MountPath: "/usr/share/fleet-server/config/http-certs",
					},
				}

				ps.Containers[0].Ports = []corev1.ContainerPort{
					{
						Name:          "https",
						ContainerPort: 8220,
						Protocol:      corev1.ProtocolTCP,
					},
				}

				ps.Containers[0].Env = []corev1.EnvVar{
					{
						Name:  "FLEET_SERVER_CERT",
						Value: "/usr/share/fleet-server/config/http-certs/tls.crt",
					},
					{
						Name:  "FLEET_SERVER_CERT_KEY",
						Value: "/usr/share/fleet-server/config/http-certs/tls.key",
					},
					{
						Name:  "FLEET_SERVER_ENABLE",
						Value: "true",
					},
					{
						Name:  "FLEET_URL",
						Value: "https://agent-agent-http.default.svc:8220",
					},
					{
						Name:  "CONFIG_PATH",
						Value: "/usr/share/elastic-agent",
					},
				}

				ps.Containers[0].Resources = corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
				}

				return ps
			}),
		},
		{
			name: "running elastic agent, without running fleet server without kb association",
			params: Params{
				Agent: agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "agent",
					},
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: false,
					},
				},
				Client: k8s.NewFakeClient(),
			},
			fleetCerts: fleetCertsFixture,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Containers[0].Env = []corev1.EnvVar{
					{
						Name:  "CONFIG_PATH",
						Value: "/usr/share/elastic-agent",
					},
				}

				ps.Containers[0].Resources = corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
				}

				return ps
			}),
		},
		{
			name: "running elastic agent, with fleet server, without es/kb association and without TLS",
			params: Params{
				Agent: agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent",
						Namespace: "default",
					},
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
				},
				Client: k8s.NewFakeClient(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-agent-http",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "http",
								Port: 8220,
							},
						},
					},
				}),
			},
			fleetCerts: fleetCertsFixture,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = nil

				ps.Containers[0].VolumeMounts = nil

				ps.Containers[0].Ports = []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 8220,
						Protocol:      corev1.ProtocolTCP,
					},
				}

				ps.Containers[0].Env = []corev1.EnvVar{
					{
						Name:  "FLEET_SERVER_ENABLE",
						Value: "true",
					},
					{
						Name:  "FLEET_SERVER_HOST",
						Value: "0.0.0.0",
					},
					{
						Name:  "FLEET_SERVER_INSECURE_HTTP",
						Value: "true",
					},
					{
						Name:  "FLEET_SERVER_PORT",
						Value: "8220",
					},
					{
						Name:  "FLEET_URL",
						Value: "http://agent-agent-http.default.svc:8220",
					},
					{
						Name:  "CONFIG_PATH",
						Value: "/usr/share/elastic-agent",
					},
				}

				ps.Containers[0].Resources = corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
					Requests: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
				}

				return ps
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := generateBuilder()
			hash := sha256.New224()

			gotBuilder, gotErr := amendBuilderForFleetMode(tt.params, tt.fleetCerts, EnrollmentAPIKey{}, builder, hash)

			require.Nil(t, gotErr)
			require.NotNil(t, gotBuilder)
			require.Equal(t, tt.wantPodSpec, gotBuilder.PodTemplate.Spec)
		})
	}
}

func Test_applyEnvVars(t *testing.T) {
	agent := agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default"},
		Spec: agentv1alpha1.AgentSpec{
			FleetServerEnabled: false,
			KibanaRef:          commonv1.ObjectSelector{Name: "kb", Namespace: "default"},
			FleetServerRef:     commonv1.ObjectSelector{Name: "fs", Namespace: "default"},
		},
	}

	agent2 := agent
	agent2.Spec.ElasticsearchRefs = []agentv1alpha1.Output{
		{
			ObjectSelector: commonv1.ObjectSelector{Name: "es", Namespace: "default"},
			OutputName:     "default",
		},
	}

	agent.GetAssociations()[0].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "kb-secret-name",
		AuthSecretKey:  "kb-user",
		CACertProvided: true,
		CASecretName:   "kb-ca-secret-name",
		URL:            "kb-url",
	})
	agent.GetAssociations()[1].SetAssociationConf(&commonv1.AssociationConf{
		URL:            "https://fs-url",
		CACertProvided: true,
	})

	agent2.Spec.FleetServerEnabled = true
	agent2.Spec.FleetServerRef = commonv1.ObjectSelector{}
	agent2.GetAssociations()[0].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "es-secret-name",
		AuthSecretKey:  "es-user",
		URL:            "es-url",
	})
	agent2.GetAssociations()[1].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "kb-secret-name",
		AuthSecretKey:  "kb-user",
		URL:            "kb-url",
	})

	podTemplateBuilderWithFleetTokenSet := generateBuilder()
	podTemplateBuilderWithFleetTokenSet = podTemplateBuilderWithFleetTokenSet.WithEnv(corev1.EnvVar{Name: "FLEET_ENROLLMENT_TOKEN", Value: "custom"})

	testToken := EnrollmentAPIKey{
		APIKey:   "test-token",
		PolicyID: "policy-id",
	}

	f := false
	for _, tt := range []struct {
		name               string
		params             Params
		fleetCerts         *certificates.CertificatesSecret
		fleetToken         EnrollmentAPIKey
		podTemplateBuilder *defaults.PodTemplateBuilder
		wantContainer      corev1.Container
		wantSecretData     map[string][]byte
	}{
		{
			name: "elastic agent, without fleet server, with fleet server ref, with kibana ref",
			params: Params{
				Context: context.Background(),
				Agent:   agent,
				Client:  k8s.NewFakeClient(),
			},
			fleetCerts:         fleetCertsFixture,
			fleetToken:         testToken,
			podTemplateBuilder: generateBuilder(),
			wantContainer: corev1.Container{
				Name: "agent",
				Env: []corev1.EnvVar{
					{Name: "FLEET_CA", Value: "/mnt/elastic-internal/fleetserver-association/default/fs/certs/ca.crt"},
					{Name: "FLEET_ENROLL", Value: "true"},
					{Name: "FLEET_ENROLLMENT_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_ENROLLMENT_TOKEN",
						Optional:             &f,
					}}},
					{Name: "FLEET_URL", Value: "https://fs-url"},
				},
			},
			wantSecretData: map[string][]byte{
				"FLEET_ENROLLMENT_TOKEN": []byte("test-token"),
			},
		},
		{
			name: "elastic agent, without fleet server, with fleet server ref, with kibana ref, token override",
			params: Params{
				Context: context.Background(),
				Agent:   agent,
				Client:  k8s.NewFakeClient(),
			},
			fleetCerts:         fleetCertsFixture,
			fleetToken:         testToken,
			podTemplateBuilder: podTemplateBuilderWithFleetTokenSet,
			wantContainer: corev1.Container{
				Name: "agent",
				Env: []corev1.EnvVar{
					{Name: "FLEET_ENROLLMENT_TOKEN", Value: "custom"},
					{Name: "FLEET_CA", Value: "/mnt/elastic-internal/fleetserver-association/default/fs/certs/ca.crt"},
					{Name: "FLEET_ENROLL", Value: "true"},
					{Name: "FLEET_URL", Value: "https://fs-url"},
				},
			},
			wantSecretData: nil,
		},
		{
			name: "elastic agent, with fleet server, with kibana ref",
			params: Params{
				Context: context.Background(),
				Agent:   agent2,
				Client: k8s.NewFakeClient(
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{Name: "agent-agent-http", Namespace: "default"},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Name: "https",
									Port: 8220,
								},
							},
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "es-secret-name", Namespace: "default"},
						Data: map[string][]byte{
							"es-user": []byte("es-password"),
						},
					},
				),
			},
			fleetCerts:         fleetCertsFixture,
			fleetToken:         testToken,
			podTemplateBuilder: generateBuilder(),
			wantContainer: corev1.Container{
				Name: "agent",
				Env: []corev1.EnvVar{
					{Name: "FLEET_CA", Value: "/usr/share/fleet-server/config/http-certs/ca.crt"},
					{Name: "FLEET_ENROLL", Value: "true"},
					{Name: "FLEET_ENROLLMENT_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_ENROLLMENT_TOKEN",
						Optional:             &f,
					}}},
					{Name: "FLEET_SERVER_CERT", Value: "/usr/share/fleet-server/config/http-certs/tls.crt"},
					{Name: "FLEET_SERVER_CERT_KEY", Value: "/usr/share/fleet-server/config/http-certs/tls.key"},
					{Name: "FLEET_SERVER_ELASTICSEARCH_HOST", Value: "es-url"},
					{Name: "FLEET_SERVER_ELASTICSEARCH_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_SERVER_ELASTICSEARCH_PASSWORD",
						Optional:             &f,
					}}},
					{Name: "FLEET_SERVER_ELASTICSEARCH_USERNAME", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_SERVER_ELASTICSEARCH_USERNAME",
						Optional:             &f,
					}}},
					{Name: "FLEET_SERVER_ENABLE", Value: "true"},
					{Name: "FLEET_SERVER_POLICY_ID", Value: "policy-id"},
					{Name: "FLEET_URL", Value: "https://agent-agent-http.default.svc:8220"},
				},
			},
			wantSecretData: map[string][]byte{
				"FLEET_ENROLLMENT_TOKEN":              []byte("test-token"),
				"FLEET_SERVER_ELASTICSEARCH_USERNAME": []byte("es-user"),
				"FLEET_SERVER_ELASTICSEARCH_PASSWORD": []byte("es-password"),
			},
		},
		{
			name: "elastic agent, with fleet server, with kibana ref with well-known CA",
			params: Params{
				Context: context.Background(),
				Agent:   agent2,
				Client: k8s.NewFakeClient(
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{Name: "agent-agent-http", Namespace: "default"},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Name: "https",
									Port: 8220,
								},
							},
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "es-secret-name", Namespace: "default"},
						Data: map[string][]byte{
							"es-user": []byte("es-password"),
						},
					},
				),
			},
			fleetCerts:         wellKnownCACertsFixture,
			fleetToken:         testToken,
			podTemplateBuilder: generateBuilder(),
			wantContainer: corev1.Container{
				Name: "agent",
				Env: []corev1.EnvVar{
					{Name: "FLEET_ENROLL", Value: "true"},
					{Name: "FLEET_ENROLLMENT_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_ENROLLMENT_TOKEN",
						Optional:             &f,
					}}},
					{Name: "FLEET_SERVER_CERT", Value: "/usr/share/fleet-server/config/http-certs/tls.crt"},
					{Name: "FLEET_SERVER_CERT_KEY", Value: "/usr/share/fleet-server/config/http-certs/tls.key"},
					{Name: "FLEET_SERVER_ELASTICSEARCH_HOST", Value: "es-url"},
					{Name: "FLEET_SERVER_ELASTICSEARCH_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_SERVER_ELASTICSEARCH_PASSWORD",
						Optional:             &f,
					}}},
					{Name: "FLEET_SERVER_ELASTICSEARCH_USERNAME", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-agent-envvars"},
						Key:                  "FLEET_SERVER_ELASTICSEARCH_USERNAME",
						Optional:             &f,
					}}},
					{Name: "FLEET_SERVER_ENABLE", Value: "true"},
					{Name: "FLEET_SERVER_POLICY_ID", Value: "policy-id"},
					{Name: "FLEET_URL", Value: "https://agent-agent-http.default.svc:8220"},
				},
			},
			wantSecretData: map[string][]byte{
				"FLEET_ENROLLMENT_TOKEN":              []byte("test-token"),
				"FLEET_SERVER_ELASTICSEARCH_USERNAME": []byte("es-user"),
				"FLEET_SERVER_ELASTICSEARCH_PASSWORD": []byte("es-password"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotBuilder, err := applyEnvVars(tt.params, tt.fleetToken, tt.fleetCerts, tt.podTemplateBuilder)

			require.NoError(t, err)

			gotContainers := gotBuilder.PodTemplate.Spec.Containers
			require.True(t, len(gotContainers) > 0)
			require.Equal(t, tt.wantContainer, gotContainers[0])

			if tt.wantSecretData == nil {
				return
			}

			var gotSecret corev1.Secret
			require.NoError(t, tt.params.Client.Get(context.Background(), types.NamespacedName{Name: "agent-agent-envvars", Namespace: "default"}, &gotSecret))
			require.Equal(t, tt.wantSecretData, gotSecret.Data)
		})
	}
}

func Test_getVolumesFromAssociations(t *testing.T) {
	// Note: we use setAssocConfs to set the AssociationConfs which are normally set in the reconciliation loop.
	for _, tt := range []struct {
		name                   string
		params                 Params
		setAssocConfs          func(assocs []commonv1.Association)
		wantAssociationsLength int
	}{
		{
			name: "fleet mode enabled, kb ref, fleet ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						Mode:      agentv1alpha1.AgentFleetMode,
						KibanaRef: commonv1.ObjectSelector{Name: "kibana"},
						ElasticsearchRefs: []agentv1alpha1.Output{
							{ObjectSelector: commonv1.ObjectSelector{Name: "elasticsearch"}, OutputName: "default"},
						},
						FleetServerRef: commonv1.ObjectSelector{Name: "fleet"},
					},
				},
			},
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "kibana-kb-http-certs-public",
				})
				assocs[1].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "fleet-agent-http-certs-public",
				})
				assocs[2].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "elasticsearch-es-ca",
				})
			},
			wantAssociationsLength: 2,
		},
		{
			name: "fleet mode enabled, kb no tls ref, fleet ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						Mode:           agentv1alpha1.AgentFleetMode,
						KibanaRef:      commonv1.ObjectSelector{Name: "kibana"},
						FleetServerRef: commonv1.ObjectSelector{Name: "fleet"},
					},
				},
			},
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					// No CASecretName
				})
				assocs[1].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "fleet-agent-http-certs-public",
				})
			},
			wantAssociationsLength: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assocs := tt.params.Agent.GetAssociations()
			tt.setAssocConfs(assocs)
			associations, err := getVolumesFromAssociations(assocs)
			require.NoError(t, err)
			require.Equal(t, tt.wantAssociationsLength, len(associations))
		})
	}
}

func Test_getRelatedEsAssoc(t *testing.T) {
	for _, tt := range []struct {
		name    string
		params  Params
		wantRef *commonv1.ObjectSelector
	}{
		{
			name: "fleet server enabled, no es ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
					},
				},
			},
			wantRef: nil,
		},
		{
			name: "fleet server enabled, es ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
						ElasticsearchRefs: []agentv1alpha1.Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "es"},
							},
						},
					},
				},
			},
			wantRef: &commonv1.ObjectSelector{Name: "es"},
		},
		{
			name: "fleet server disabled, no fs ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: false,
					},
				},
			},
			wantRef: nil,
		},
		{
			name: "fleet server disabled, fs ref, no es ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: false,
						FleetServerRef:     commonv1.ObjectSelector{Name: "fs"},
					},
				},
				Context: context.Background(),
				Client: k8s.NewFakeClient(&agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fs",
					},
				}),
			},
			wantRef: nil,
		},
		{
			name: "fleet server disabled, fs ref, es ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: false,
						FleetServerRef:     commonv1.ObjectSelector{Name: "fs"},
					},
				},
				Context: context.Background(),
				Client: k8s.NewFakeClient(&agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fs",
					},
					Spec: agentv1alpha1.AgentSpec{
						ElasticsearchRefs: []agentv1alpha1.Output{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "es"},
							},
						},
					},
				}),
			},
			wantRef: &commonv1.ObjectSelector{Name: "es"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotAssoc, gotErr := getRelatedEsAssoc(tt.params)
			require.Nil(t, gotErr)

			if tt.wantRef == nil {
				require.Nil(t, gotAssoc)
			} else {
				require.NotNil(t, gotAssoc)
				require.Equal(t, *tt.wantRef, gotAssoc.AssociationRef())
			}
		})
	}
}

func Test_applyRelatedEsAssoc(t *testing.T) {
	optional := false
	agentNs := "agent-ns"
	assocToSameNs := (&agentv1alpha1.Agent{
		Spec: agentv1alpha1.AgentSpec{
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					ObjectSelector: commonv1.ObjectSelector{
						Name:      "elasticsearch",
						Namespace: agentNs,
					},
				},
			},
		},
	}).GetAssociations()[0]
	assocToSameNs.SetAssociationConf(&commonv1.AssociationConf{
		CASecretName: "elasticsearch-es-http-certs-public",
	})

	assocToOtherNs := (&agentv1alpha1.Agent{
		Spec: agentv1alpha1.AgentSpec{
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					ObjectSelector: commonv1.ObjectSelector{
						Name:      "elasticsearch",
						Namespace: "elasticsearch-ns",
					},
				},
			},
		},
	}).GetAssociations()[0]
	assocToOtherNs.SetAssociationConf(&commonv1.AssociationConf{
		CASecretName: "elasticsearch-es-http-certs-public",
	})

	expectedCAVolume := []corev1.Volume{
		{
			Name: "elasticsearch-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "elasticsearch-es-http-certs-public",
					Optional:   &optional,
				},
			},
		},
	}
	expectedCAVolumeMountFunc := func(ns string) []corev1.VolumeMount {
		return []corev1.VolumeMount{
			{
				Name:      "elasticsearch-certs",
				ReadOnly:  true,
				MountPath: fmt.Sprintf("/mnt/elastic-internal/elasticsearch-association/%s/elasticsearch/certs", ns),
			},
		}
	}
	expectedCmdFunc := func(ns string) []string {
		return []string{"/usr/bin/env", "bash", "-c", fmt.Sprintf(`#!/usr/bin/env bash
set -e
if [[ -f /mnt/elastic-internal/elasticsearch-association/%[1]s/elasticsearch/certs/ca.crt ]]; then
  if [[ -f /usr/bin/update-ca-trust ]]; then
    cp /mnt/elastic-internal/elasticsearch-association/%[1]s/elasticsearch/certs/ca.crt /etc/pki/ca-trust/source/anchors/
    /usr/bin/update-ca-trust
  elif [[ -f /usr/sbin/update-ca-certificates ]]; then
    cp /mnt/elastic-internal/elasticsearch-association/%[1]s/elasticsearch/certs/ca.crt /usr/local/share/ca-certificates/
    /usr/sbin/update-ca-certificates
  fi
fi
/usr/bin/tini -- /usr/local/bin/docker-entrypoint -e
`, ns)}
	}
	for _, tt := range []struct {
		name        string
		agent       agentv1alpha1.Agent
		assoc       commonv1.Association
		wantPodSpec corev1.PodSpec
		wantErr     bool
	}{
		{
			name:        "nil es association",
			wantPodSpec: generateBuilder().PodTemplate.Spec,
			wantErr:     false,
		},
		{
			name: "fleet server disabled, same namespace",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: agentNs,
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "7.16.2",
					FleetServerEnabled: false,
					DaemonSet: &agentv1alpha1.DaemonSetSpec{
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "agent",
										SecurityContext: &corev1.SecurityContext{
											RunAsUser: ptr.To[int64](0),
										},
									},
								},
							},
						},
					},
				},
			},
			assoc:   assocToSameNs,
			wantErr: false,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = expectedCAVolume
				ps.Containers[0].VolumeMounts = expectedCAVolumeMountFunc(agentNs)
				ps.Containers[0].Command = expectedCmdFunc(agentNs)
				return ps
			}),
		},
		{
			name: "fleet server enabled 8x has volumes and volumeMount but no ca-init command",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: agentNs,
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
				},
			},
			assoc:   assocToSameNs,
			wantErr: false,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = expectedCAVolume
				ps.Containers[0].VolumeMounts = expectedCAVolumeMountFunc(agentNs)
				ps.Containers[0].Command = nil
				return ps
			}),
		},
		{
			name: "fleet server disabled, different namespace still has volumes and volumeMount configured",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: agentNs,
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "7.16.2",
					FleetServerEnabled: false,
					DaemonSet: &agentv1alpha1.DaemonSetSpec{
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "agent",
										SecurityContext: &corev1.SecurityContext{
											RunAsUser: ptr.To[int64](0),
										},
									},
								},
							},
						},
					},
				},
			},
			assoc:   assocToOtherNs,
			wantErr: false,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = expectedCAVolume
				ps.Containers[0].VolumeMounts = expectedCAVolumeMountFunc("elasticsearch-ns")
				ps.Containers[0].Command = expectedCmdFunc("elasticsearch-ns")
				return ps
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := generateBuilder()
			gotBuilder, gotErr := applyRelatedEsAssoc(tt.agent, tt.assoc, builder)
			require.Equal(t, tt.wantErr, gotErr != nil)
			if !tt.wantErr {
				require.Nil(t, gotErr)
				require.Nil(t, deep.Equal(tt.wantPodSpec, gotBuilder.PodTemplate.Spec))
			}
		})
	}
}

func Test_writeEsAssocToConfigHash(t *testing.T) {
	assoc := (&agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent",
			Namespace: "ns",
		},
		Spec: agentv1alpha1.AgentSpec{
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					ObjectSelector: commonv1.ObjectSelector{
						Name:      "es",
						Namespace: "ns",
					},
				},
			},
		},
	}).GetAssociations()[0]

	assoc.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "auth-secret-name",
		AuthSecretKey:  "auth-secret-key",
		CASecretName:   "ca-secret-name",
	})

	for _, tt := range []struct {
		name           string
		params         Params
		assoc          commonv1.Association
		wantHashChange bool
	}{
		{
			name:           "nil association",
			wantHashChange: false,
		},
		{
			name: "fleet server enabled",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
					},
				},
			},
			wantHashChange: false,
		},
		{
			name: "fleet server disabled, expect hash to be changed",
			params: Params{
				Client: k8s.NewFakeClient(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "auth-secret-name",
							Namespace: "ns",
						},
						Data: map[string][]byte{
							"auth-secret-key": []byte("abc"),
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ca-secret-name",
							Namespace: "ns",
						},
						Data: map[string][]byte{
							"ca.crt": []byte("def"),
						},
					},
				),
			},
			assoc:          assoc,
			wantHashChange: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			baseHashSum := sha256.New224().Sum(nil)
			hash := sha256.New224()

			err := writeEsAssocToConfigHash(tt.params, tt.assoc, hash)

			require.Nil(t, err)
			require.Equal(t, tt.wantHashChange, !bytes.Equal(baseHashSum, hash.Sum(nil)))
		})
	}
}

func Test_getFleetSetupKibanaEnvVars(t *testing.T) {
	agent := agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "agent",
		},
		Spec: agentv1alpha1.AgentSpec{
			KibanaRef: commonv1.ObjectSelector{
				Name:      "kibana",
				Namespace: "ns",
			},
		},
	}

	for _, tt := range []struct {
		name        string
		agent       agentv1alpha1.Agent
		fleetToken  EnrollmentAPIKey
		wantErr     bool
		wantEnvVars map[string]string
	}{
		{
			name:        "no kibana ref",
			agent:       agentv1alpha1.Agent{},
			wantEnvVars: map[string]string{},
			wantErr:     false,
		},
		{
			name:        "kibana ref present, but no token",
			agent:       agent,
			fleetToken:  EnrollmentAPIKey{},
			wantEnvVars: nil,
			wantErr:     true,
		},
		{
			name:       "kibana ref present, token populated",
			agent:      agent,
			fleetToken: EnrollmentAPIKey{APIKey: "test-token"},
			wantEnvVars: map[string]string{
				"FLEET_ENROLLMENT_TOKEN": "test-token",
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotEnvVars, gotErr := getFleetSetupKibanaEnvVars(tt.fleetToken)(tt.agent)

			require.Equal(t, tt.wantEnvVars, gotEnvVars)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func Test_getFleetSetupFleetEnvVars(t *testing.T) {
	assoc := &agentv1alpha1.AgentFleetServerAssociation{
		Agent: &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent",
				Namespace: "ns",
			},
			Spec: agentv1alpha1.AgentSpec{
				FleetServerRef: commonv1.ObjectSelector{
					Name:      "fleet-server",
					Namespace: "ns",
				},
			},
		},
	}

	assoc.SetAssociationConf(&commonv1.AssociationConf{
		URL:            "https://fleet-server",
		CACertProvided: true,
	})

	assocNoTLS := &agentv1alpha1.AgentFleetServerAssociation{
		Agent: &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent",
				Namespace: "ns",
			},
			Spec: agentv1alpha1.AgentSpec{
				FleetServerRef: commonv1.ObjectSelector{
					Name:      "fleet-server",
					Namespace: "ns",
				},
			},
		},
	}

	assocNoTLS.SetAssociationConf(&commonv1.AssociationConf{
		URL:            "http://fleet-server",
		CACertProvided: false,
	})

	assocWithKibanaRef := &agentv1alpha1.AgentFleetServerAssociation{
		Agent: &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent",
				Namespace: "ns",
			},
			Spec: agentv1alpha1.AgentSpec{
				FleetServerRef: commonv1.ObjectSelector{
					Name:      "fleet-server",
					Namespace: "ns",
				},
				KibanaRef: commonv1.ObjectSelector{
					Name:      "kibana",
					Namespace: "ns",
				},
			},
		},
	}

	assocWithKibanaRef.SetAssociationConf(&commonv1.AssociationConf{
		URL:            "https://fleet-server",
		CACertProvided: true,
	})

	for _, tt := range []struct {
		name        string
		agent       agentv1alpha1.Agent
		fleetCerts  *certificates.CertificatesSecret
		wantErr     bool
		wantEnvVars map[string]string
		client      k8s.Client
	}{
		{
			name: "fleet server enabled, kibana ref",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: "ns",
				},
				Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					KibanaRef: commonv1.ObjectSelector{
						Name:      "kibana",
						Namespace: "ns",
					},
				},
			},
			fleetCerts: fleetCertsFixture,
			wantErr:    false,
			wantEnvVars: map[string]string{
				"FLEET_ENROLL": "true",
				"FLEET_CA":     "/usr/share/fleet-server/config/http-certs/ca.crt",
				"FLEET_URL":    "https://agent-agent-http.ns.svc:8220",
			},
			client: k8s.NewFakeClient(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "agent-agent-http",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:        "https",
							Protocol:    "",
							AppProtocol: nil,
							Port:        8220,
						},
					},
				},
			}),
		},
		{
			name: "fleet server enabled, kibana ref, well known CA",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: "ns",
				},
				Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
					KibanaRef: commonv1.ObjectSelector{
						Name:      "kibana",
						Namespace: "ns",
					},
				},
			},
			fleetCerts: wellKnownCACertsFixture,
			wantErr:    false,
			wantEnvVars: map[string]string{
				"FLEET_ENROLL": "true",
				"FLEET_URL":    "https://agent-agent-http.ns.svc:8220",
			},
			client: k8s.NewFakeClient(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "agent-agent-http",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:        "https",
							Protocol:    "",
							AppProtocol: nil,
							Port:        8220,
						},
					},
				},
			}),
		},
		{
			name: "fleet server enabled, no kibana ref",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: "ns",
				},
				Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
				},
			},
			fleetCerts: fleetCertsFixture,
			wantErr:    false,
			wantEnvVars: map[string]string{
				"FLEET_CA":  "/usr/share/fleet-server/config/http-certs/ca.crt",
				"FLEET_URL": "https://agent-agent-http.ns.svc:8220",
			},
			client: k8s.NewFakeClient(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "agent-agent-http",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "https",
							Port: 8220,
						},
					},
				},
			}),
		},
		{
			name:    "fleet server not enabled, fleet server ref, no kibana ref",
			agent:   *assoc.Agent,
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_CA":  "/mnt/elastic-internal/fleetserver-association/ns/fleet-server/certs/ca.crt",
				"FLEET_URL": "https://fleet-server",
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:    "fleet server not enabled, fleet server ref, kibana ref",
			agent:   *assocWithKibanaRef.Agent,
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_ENROLL": "true",
				"FLEET_CA":     "/mnt/elastic-internal/fleetserver-association/ns/fleet-server/certs/ca.crt",
				"FLEET_URL":    "https://fleet-server",
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:    "fleet server not enabled, fleet server ref no tls, no kibana ref",
			agent:   *assocNoTLS.Agent,
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_INSECURE": "true",
				"FLEET_URL":      "http://fleet-server",
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:        "fleet server not enabled, no fleet server ref, kibana ref",
			agent:       agentv1alpha1.Agent{},
			wantErr:     false,
			wantEnvVars: map[string]string{},
		},
		{
			name: "fleet server not enabled, no fleet server ref, kibana ref",
			agent: agentv1alpha1.Agent{
				Spec: agentv1alpha1.AgentSpec{
					KibanaRef: commonv1.ObjectSelector{
						Name:      "kibana",
						Namespace: "ns",
					},
				},
			},
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_ENROLL": "true",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotEnvVars, gotErr := getFleetSetupFleetEnvVars(tt.client, EnrollmentAPIKey{}, tt.fleetCerts)(tt.agent)

			require.Equal(t, tt.wantEnvVars, gotEnvVars)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func Test_getFleetSetupFleetServerEnvVars(t *testing.T) {
	agentWithoutCa := agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent",
			Namespace: "ns",
		},
		Spec: agentv1alpha1.AgentSpec{
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					ObjectSelector: commonv1.ObjectSelector{
						Name:      "es",
						Namespace: "es-ns",
					},
				},
			},
			FleetServerEnabled: true,
		},
	}
	agentWithCa := agentWithoutCa

	assocWithoutCa := agentWithoutCa.GetAssociations()[0]
	assocWithoutCa.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
	})

	assocWithCa := agentWithCa.GetAssociations()[0]
	assocWithCa.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
		CACertProvided: true,
		CASecretName:   "ca-secret-name",
	})

	for _, tt := range []struct {
		name        string
		agent       agentv1alpha1.Agent
		wantErr     bool
		wantEnvVars map[string]string
		client      k8s.Client
	}{
		{
			name:        "fleet server disabled",
			agent:       agentv1alpha1.Agent{},
			wantErr:     false,
			wantEnvVars: map[string]string{},
			client:      nil,
		},
		{
			name: "fleet server enabled, no elasticsearch ref",
			agent: agentv1alpha1.Agent{
				Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
				},
			},
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_SERVER_ENABLE":   "true",
				"FLEET_SERVER_CERT":     path.Join(FleetCertsMountPath, certificates.CertFileName),
				"FLEET_SERVER_CERT_KEY": path.Join(FleetCertsMountPath, certificates.KeyFileName),
			},
			client: nil,
		},
		{
			name:    "fleet server enabled, elasticsearch ref, no elasticsearch ca",
			agent:   agentWithoutCa,
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_SERVER_ENABLE":                 "true",
				"FLEET_SERVER_CERT":                   path.Join(FleetCertsMountPath, certificates.CertFileName),
				"FLEET_SERVER_CERT_KEY":               path.Join(FleetCertsMountPath, certificates.KeyFileName),
				"FLEET_SERVER_ELASTICSEARCH_HOST":     "url",
				"FLEET_SERVER_ELASTICSEARCH_USERNAME": "user",
				"FLEET_SERVER_ELASTICSEARCH_PASSWORD": "password",
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "ns",
				},
				Data: map[string][]byte{
					"user": []byte("password"),
				},
			}),
		},
		{
			name:    "fleet server enabled, elasticsearch ref, elasticsearch ca populated",
			agent:   agentWithCa,
			wantErr: false,
			wantEnvVars: map[string]string{
				"FLEET_SERVER_ENABLE":                 "true",
				"FLEET_SERVER_CERT":                   path.Join(FleetCertsMountPath, certificates.CertFileName),
				"FLEET_SERVER_CERT_KEY":               path.Join(FleetCertsMountPath, certificates.KeyFileName),
				"FLEET_SERVER_ELASTICSEARCH_HOST":     "url",
				"FLEET_SERVER_ELASTICSEARCH_USERNAME": "user",
				"FLEET_SERVER_ELASTICSEARCH_PASSWORD": "password",
				"FLEET_SERVER_ELASTICSEARCH_CA":       "/mnt/elastic-internal/elasticsearch-association/es-ns/es/certs/ca.crt",
			},
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "ns",
				},
				Data: map[string][]byte{
					"user": []byte("password"),
				},
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotEnvVars, gotErr := getFleetSetupFleetServerEnvVars(context.Background(), tt.client)(tt.agent)

			require.Equal(t, tt.wantEnvVars, gotEnvVars)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func generateBuilder() *defaults.PodTemplateBuilder {
	return defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, "agent")
}

func generatePodSpec(f func(spec corev1.PodSpec) corev1.PodSpec) corev1.PodSpec {
	builder := generateBuilder()
	return f(builder.PodTemplate.Spec)
}
