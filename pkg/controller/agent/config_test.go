// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package agent

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestBuildFleetSetupKibanaConfig(t *testing.T) {
	client := k8s.NewFakeClient(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "secret-name",
		},
		Data: map[string][]byte{
			"user": []byte("password"),
		},
	})

	assoc := &agentv1alpha1.AgentKibanaAssociation{
		Agent: &agentv1alpha1.Agent{
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
		},
	}

	assoc.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
	})

	for _, tt := range []struct {
		name    string
		agent   agentv1alpha1.Agent
		wantErr bool
		wantCfg map[string]interface{}
		client  k8s.Client
	}{
		{
			name:    "no kibana ref",
			agent:   agentv1alpha1.Agent{},
			wantCfg: nil,
			wantErr: false,
			client:  client,
		},
		{
			name:  "kibana ref present",
			agent: *assoc.Agent,
			wantCfg: map[string]interface{}{
				"fleet": map[string]interface{}{
					"ca":       "/mnt/elastic-internal/kibana-association/ns/kibana/certs/ca.crt",
					"host":     "url",
					"password": "password",
					"setup":    true,
					"username": "user",
				},
			},
			wantErr: false,
			client:  client,
		},
		{
			name:    "no user secret",
			agent:   *assoc.Agent,
			wantCfg: nil,
			wantErr: true,
			client:  k8s.NewFakeClient(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotCfg, gotErr := buildFleetSetupKibanaConfig(tt.agent, tt.client)

			require.Equal(t, tt.wantCfg, gotCfg)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func TestBuildFleetSetupFleetConfig(t *testing.T) {
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
		name    string
		agent   agentv1alpha1.Agent
		wantErr bool
		wantCfg map[string]interface{}
		client  k8s.Client
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
			wantCfg: map[string]interface{}{
				"enroll": true,
				"ca":     "/usr/share/fleet-server/config/http-certs/ca.crt",
				"url":    "https://agent-agent-http.ns.svc:8220",
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
			wantCfg: map[string]interface{}{
				"enroll": true,
				"ca":     "/usr/share/fleet-server/config/http-certs/ca.crt",
				"url":    "https://agent-agent-http.ns.svc:8220",
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
			wantCfg: map[string]interface{}{
				"enroll": false,
				"ca":     "/mnt/elastic-internal/fleetserver-association/ns/fleet-server/certs/ca.crt",
				"url":    "url",
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:    "fleet server not enabled, fleet server ref, kibana ref",
			agent:   *assocWithKibanaRef.Agent,
			wantErr: false,
			wantCfg: map[string]interface{}{
				"enroll": true,
				"ca":     "/mnt/elastic-internal/fleetserver-association/ns/fleet-server/certs/ca.crt",
				"url":    "url",
			},
			client: k8s.NewFakeClient(),
		},
		{
			name:    "fleet server not enabled, no fleet server ref, kibana ref",
			agent:   agentv1alpha1.Agent{},
			wantErr: false,
			wantCfg: map[string]interface{}{
				"enroll": false,
			},
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
			wantCfg: map[string]interface{}{
				"enroll": true,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotCfg, gotErr := buildFleetSetupFleetConfig(tt.agent, tt.client)

			require.Equal(t, tt.wantCfg, gotCfg)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func TestBuildFleetSetupFleetServerConfig(t *testing.T) {
	agent := agentv1alpha1.Agent{
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

	assoc := agent.GetAssociations()[0]
	assoc.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
	})

	for _, tt := range []struct {
		name    string
		agent   agentv1alpha1.Agent
		wantErr bool
		wantCfg map[string]interface{}
		client  k8s.Client
	}{
		{
			name:    "fleet server disabled",
			agent:   agentv1alpha1.Agent{},
			wantErr: false,
			wantCfg: map[string]interface{}{"enable": false},
			client:  nil,
		},
		{
			name: "fleet server enabled, no elasticsearch ref",
			agent: agentv1alpha1.Agent{
				Spec: agentv1alpha1.AgentSpec{
					FleetServerEnabled: true,
				},
			},
			wantErr: false,
			wantCfg: map[string]interface{}{
				"enable":   true,
				"cert":     path.Join(FleetCertsMountPath, certificates.CertFileName),
				"cert_key": path.Join(FleetCertsMountPath, certificates.KeyFileName),
			},
			client: nil,
		},
		{
			name:    "fleet server enabled, elasticsearch ref",
			agent:   agent,
			wantErr: false,
			wantCfg: map[string]interface{}{
				"enable":   true,
				"cert":     path.Join(FleetCertsMountPath, certificates.CertFileName),
				"cert_key": path.Join(FleetCertsMountPath, certificates.KeyFileName),
				"elasticsearch": map[string]interface{}{
					"ca":       "/mnt/elastic-internal/elasticsearch-association/es-ns/es/certs/ca.crt",
					"host":     "url",
					"username": "user",
					"password": "password",
				},
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
			gotCfg, gotErr := buildFleetSetupFleetServerConfig(tt.agent, tt.client)

			require.Equal(t, tt.wantCfg, gotCfg)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func TestExtractConnectionSettings(t *testing.T) {
	assoc := agentv1alpha1.AgentKibanaAssociation{
		Agent: &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent",
				Namespace: "ns",
			},
			Spec: agentv1alpha1.AgentSpec{
				KibanaRef: commonv1.ObjectSelector{
					Name:      "kibana",
					Namespace: "ns",
				},
			},
		},
	}

	assoc.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
	})

	for _, tt := range []struct {
		name                                         string
		agent                                        agentv1alpha1.Agent
		client                                       k8s.Client
		assocType                                    commonv1.AssociationType
		wantHost, wantCA, wantUsername, wantPassword string
		wantErr                                      bool
	}{
		{
			name:      "no association of this type",
			agent:     agentv1alpha1.Agent{},
			client:    nil,
			assocType: commonv1.KibanaAssociationType,
			wantErr:   true,
		},
		{
			name:      "no auth secret",
			agent:     *assoc.Agent,
			client:    k8s.NewFakeClient(),
			assocType: commonv1.KibanaAssociationType,
			wantErr:   true,
		},
		{
			name:  "happy path",
			agent: *assoc.Agent,
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "ns",
				},
				Data: map[string][]byte{
					"user": []byte("password"),
				},
			}),
			assocType:    commonv1.KibanaAssociationType,
			wantHost:     "url",
			wantCA:       "/mnt/elastic-internal/kibana-association/ns/kibana/certs/ca.crt",
			wantUsername: "user",
			wantPassword: "password",
			wantErr:      false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotCA, gotUsername, gotPassword, gotErr := extractConnectionSettings(tt.agent, tt.client, tt.assocType)

			require.Equal(t, tt.wantHost, gotHost)
			require.Equal(t, tt.wantCA, gotCA)
			require.Equal(t, tt.wantUsername, gotUsername)
			require.Equal(t, tt.wantPassword, gotPassword)

			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}
