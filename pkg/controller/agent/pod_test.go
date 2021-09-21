// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
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
						Name: "agent",
					},
					Spec: agentv1alpha1.AgentSpec{
						FleetServerEnabled: true,
					},
				},
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
					{
						Name: "fleet-setup-config",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  "agent-agent-config",
								DefaultMode: pointer.Int32(0440),
								Optional:    &optional,
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
					{
						Name:      "fleet-setup-config",
						ReadOnly:  true,
						MountPath: "/usr/share/elastic-agent/fleet-setup.yml",
						SubPath:   "fleet-setup.yml",
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
				ps.Volumes = []corev1.Volume{
					{
						Name: "fleet-setup-config",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  "agent-agent-config",
								DefaultMode: pointer.Int32(0440),
								Optional:    &optional,
							},
						},
					},
				}

				ps.Containers[0].VolumeMounts = []corev1.VolumeMount{
					{
						Name:      "fleet-setup-config",
						ReadOnly:  true,
						MountPath: "/usr/share/elastic-agent/fleet-setup.yml",
						SubPath:   "fleet-setup.yml",
					},
				}

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
		setAssocConfs          func(assocs []v1.Association)
		wantAssociationsLength int
	}{
		{
			name: "fleet mode enabled, kb ref, fleet ref",
			params: Params{
				Agent: agentv1alpha1.Agent{
					Spec: agentv1alpha1.AgentSpec{
						Mode:           agentv1alpha1.AgentFleetMode,
						KibanaRef:      v1.ObjectSelector{Name: "kibana"},
						FleetServerRef: v1.ObjectSelector{Name: "fleet"},
					},
				},
			},
			setAssocConfs: func(assocs []v1.Association) {
				assocs[0].SetAssociationConf(&v1.AssociationConf{
					CASecretName: "kibana-kb-http-certs-public",
				})
				assocs[1].SetAssociationConf(&v1.AssociationConf{
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
						KibanaRef:      v1.ObjectSelector{Name: "kibana"},
						FleetServerRef: v1.ObjectSelector{Name: "fleet"},
					},
				},
			},
			setAssocConfs: func(assocs []v1.Association) {
				assocs[0].SetAssociationConf(&v1.AssociationConf{
					// No CASecretName
				})
				assocs[1].SetAssociationConf(&v1.AssociationConf{
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
		wantRef *v1.ObjectSelector
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
								ObjectSelector: v1.ObjectSelector{Name: "es"},
							},
						},
					},
				},
			},
			wantRef: &v1.ObjectSelector{Name: "es"},
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
						FleetServerRef:     v1.ObjectSelector{Name: "fs"},
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
						FleetServerRef:     v1.ObjectSelector{Name: "fs"},
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
								ObjectSelector: v1.ObjectSelector{Name: "es"},
							},
						},
					},
				}),
			},
			wantRef: &v1.ObjectSelector{Name: "es"},
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
					ObjectSelector: v1.ObjectSelector{
						Name:      "elasticsearch",
						Namespace: agentNs,
					},
				},
			},
		},
	}).GetAssociations()[0]
	assocToSameNs.SetAssociationConf(&v1.AssociationConf{
		CASecretName: "elasticsearch-es-http-certs-public",
	})

	assocToOtherNs := (&agentv1alpha1.Agent{
		Spec: agentv1alpha1.AgentSpec{
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					ObjectSelector: v1.ObjectSelector{
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
		assoc       v1.Association
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
					ObjectSelector: v1.ObjectSelector{
						Name:      "es",
						Namespace: "ns",
					},
				},
			},
		},
	}).GetAssociations()[0]

	assoc.SetAssociationConf(&v1.AssociationConf{
		AuthSecretName: "auth-secret-name",
		AuthSecretKey:  "auth-secret-key",
		CASecretName:   "ca-secret-name",
	})

	for _, tt := range []struct {
		name           string
		params         Params
		assoc          v1.Association
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

func generateBuilder() *defaults.PodTemplateBuilder {
	return defaults.NewPodTemplateBuilder(corev1.PodTemplateSpec{}, "agent")
}

func generatePodSpec(f func(spec corev1.PodSpec) corev1.PodSpec) corev1.PodSpec {
	builder := generateBuilder()
	return f(builder.PodTemplate.Spec)
}
