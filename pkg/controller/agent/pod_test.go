// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_amendBuilderForFleetMode(t *testing.T) {
	optional := false

	for _, tt := range []struct {
		name        string
		params      Params
		wantPodSpec corev1.PodSpec
	}{
		{
			name: "running fleet server without es association",
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
						Name:  "FLEET_URL",
						Value: "https://agent-agent-http.default.svc:8220",
					},
					{
						Name:  "FLEET_SERVER_ENABLE",
						Value: "true",
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
			name: "not running fleet server, no fleet server ref", params: Params{
				Agent: agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "agent",
					},
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: false,
					},
				},
			},
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			fleetCerts := &certificates.CertificatesSecret{
				Secret: corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "fleet-certs-secret-name",
					},
				},
			}
			builder := generateBuilder()
			hash := sha256.New224()

			gotBuilder, gotErr := amendBuilderForFleetMode(tt.params, fleetCerts, builder, hash)

			require.Nil(t, gotErr)
			require.NotNil(t, gotBuilder)
			require.Equal(t, tt.wantPodSpec, gotBuilder.PodTemplate.Spec)
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
						Mode:           agentv1alpha1.AgentFleetMode,
						KibanaRef:      commonv1.ObjectSelector{Name: "kibana"},
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
			associations := getVolumesFromAssociations(assocs)
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
					FleetServerEnabled: false,
				},
			},
			assoc:   assocToSameNs,
			wantErr: false,
			wantPodSpec: generatePodSpec(func(ps corev1.PodSpec) corev1.PodSpec {
				ps.Volumes = []corev1.Volume{
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

				ps.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "elasticsearch-certs",
						ReadOnly:  true,
						MountPath: "/mnt/elastic-internal/elasticsearch-association/agent-ns/elasticsearch/certs",
					},
				}

				ps.Containers[0].Command = []string{"/usr/bin/env", "bash", "-c", `#!/usr/bin/env bash
set -e
if [[ -f /mnt/elastic-internal/elasticsearch-association/agent-ns/elasticsearch/certs/ca.crt ]]; then
  cp /mnt/elastic-internal/elasticsearch-association/agent-ns/elasticsearch/certs/ca.crt /etc/pki/ca-trust/source/anchors/
  update-ca-trust
fi
/usr/bin/tini -- /usr/local/bin/docker-entrypoint -e
`}

				return ps
			}),
		},
		{
			name: "fleet server disabled, different namespace",
			agent: agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent",
					Namespace: agentNs,
				},
				Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: false,
				},
			},
			assoc:   assocToOtherNs,
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := generateBuilder()
			gotBuilder, gotErr := applyRelatedEsAssoc(tt.agent, tt.assoc, builder)

			require.Equal(t, tt.wantErr, gotErr != nil)
			if !tt.wantErr {
				require.Nil(t, gotErr)
				require.Equal(t, tt.wantPodSpec, gotBuilder.PodTemplate.Spec)
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
							"tls.crt": []byte("def"),
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
	client := k8s.NewFakeClient(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "secret-name",
		},
		Data: map[string][]byte{
			"user": []byte("password"),
		},
	})
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
	agent2 := agent

	assocWithoutCa := &agentv1alpha1.AgentKibanaAssociation{
		Agent: &agent,
	}

	assocWithCa := &agentv1alpha1.AgentKibanaAssociation{
		Agent: &agent2,
	}

	assocWithoutCa.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
	})

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
		wantEnvVars []corev1.EnvVar
		client      k8s.Client
	}{
		{
			name:        "no kibana ref",
			agent:       agentv1alpha1.Agent{},
			wantEnvVars: []corev1.EnvVar{},
			wantErr:     false,
			client:      client,
		},
		{
			name:  "kibana ref present, kibana without ca populated",
			agent: *assocWithoutCa.Agent,
			wantEnvVars: []corev1.EnvVar{
				{Name: "KIBANA_FLEET_HOST", Value: "url"},
				{Name: "KIBANA_FLEET_USERNAME", Value: "user"},
				{Name: "KIBANA_FLEET_PASSWORD", Value: "password"},
				{Name: "KIBANA_FLEET_SETUP", Value: "true"},
			},
			wantErr: false,
			client:  client,
		},
		{
			name:  "kibana ref present, kibana with ca populated",
			agent: *assocWithCa.Agent,
			wantEnvVars: []corev1.EnvVar{
				{Name: "KIBANA_FLEET_HOST", Value: "url"},
				{Name: "KIBANA_FLEET_USERNAME", Value: "user"},
				{Name: "KIBANA_FLEET_PASSWORD", Value: "password"},
				{Name: "KIBANA_FLEET_SETUP", Value: "true"},
				{Name: "KIBANA_FLEET_CA", Value: "/mnt/elastic-internal/kibana-association/ns/kibana/certs/ca.crt"},
			},
			wantErr: false,
			client:  client,
		},
		{
			name:        "no user secret",
			agent:       *assocWithoutCa.Agent,
			wantEnvVars: nil,
			wantErr:     true,
			client:      k8s.NewFakeClient(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotEnvVars, gotErr := getFleetSetupKibanaEnvVars(tt.agent, tt.client)

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
		URL: "url",
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
		URL: "url",
	})

	for _, tt := range []struct {
		name        string
		agent       agentv1alpha1.Agent
		wantErr     bool
		wantEnvVars []corev1.EnvVar
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
			wantErr: false,
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_ENROLL", Value: "true"},
				{Name: "FLEET_CA", Value: "/usr/share/fleet-server/config/http-certs/ca.crt"},
				{Name: "FLEET_URL", Value: "https://agent-agent-http.ns.svc:8220"},
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
					KibanaRef: commonv1.ObjectSelector{
						Name:      "kibana",
						Namespace: "ns",
					},
				},
			},
			wantErr: false,
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_ENROLL", Value: "true"},
				{Name: "FLEET_CA", Value: "/usr/share/fleet-server/config/http-certs/ca.crt"},
				{Name: "FLEET_URL", Value: "https://agent-agent-http.ns.svc:8220"},
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
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_CA", Value: "/mnt/elastic-internal/fleetserver-association/ns/fleet-server/certs/ca.crt"},
				{Name: "FLEET_URL", Value: "url"},
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:    "fleet server not enabled, fleet server ref, kibana ref",
			agent:   *assocWithKibanaRef.Agent,
			wantErr: false,
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_ENROLL", Value: "true"},
				{Name: "FLEET_CA", Value: "/mnt/elastic-internal/fleetserver-association/ns/fleet-server/certs/ca.crt"},
				{Name: "FLEET_URL", Value: "url"},
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:        "fleet server not enabled, no fleet server ref, kibana ref",
			agent:       agentv1alpha1.Agent{},
			wantErr:     false,
			wantEnvVars: []corev1.EnvVar{},
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
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_ENROLL", Value: "true"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotEnvVars, gotErr := getFleetSetupFleetEnvVars(tt.agent, tt.client)

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
		wantEnvVars []corev1.EnvVar
		client      k8s.Client
	}{
		{
			name:        "fleet server disabled",
			agent:       agentv1alpha1.Agent{},
			wantErr:     false,
			wantEnvVars: []corev1.EnvVar{},
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
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_SERVER_ENABLE", Value: "true"},
				{Name: "FLEET_SERVER_CERT", Value: path.Join(FleetCertsMountPath, certificates.CertFileName)},
				{Name: "FLEET_SERVER_CERT_KEY", Value: path.Join(FleetCertsMountPath, certificates.KeyFileName)},
			},
			client: nil,
		},
		{
			name:    "fleet server enabled, elasticsearch ref, no elasticsearch ca",
			agent:   agentWithoutCa,
			wantErr: false,
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_SERVER_ENABLE", Value: "true"},
				{Name: "FLEET_SERVER_CERT", Value: path.Join(FleetCertsMountPath, certificates.CertFileName)},
				{Name: "FLEET_SERVER_CERT_KEY", Value: path.Join(FleetCertsMountPath, certificates.KeyFileName)},
				{Name: "FLEET_SERVER_ELASTICSEARCH_HOST", Value: "url"},
				{Name: "FLEET_SERVER_ELASTICSEARCH_USERNAME", Value: "user"},
				{Name: "FLEET_SERVER_ELASTICSEARCH_PASSWORD", Value: "password"},
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
			wantEnvVars: []corev1.EnvVar{
				{Name: "FLEET_SERVER_ENABLE", Value: "true"},
				{Name: "FLEET_SERVER_CERT", Value: path.Join(FleetCertsMountPath, certificates.CertFileName)},
				{Name: "FLEET_SERVER_CERT_KEY", Value: path.Join(FleetCertsMountPath, certificates.KeyFileName)},
				{Name: "FLEET_SERVER_ELASTICSEARCH_HOST", Value: "url"},
				{Name: "FLEET_SERVER_ELASTICSEARCH_USERNAME", Value: "user"},
				{Name: "FLEET_SERVER_ELASTICSEARCH_PASSWORD", Value: "password"},
				{Name: "FLEET_SERVER_ELASTICSEARCH_CA", Value: "/mnt/elastic-internal/elasticsearch-association/es-ns/es/certs/ca.crt"},
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
			gotEnvVars, gotErr := getFleetSetupFleetServerEnvVars(tt.agent, tt.client)

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
