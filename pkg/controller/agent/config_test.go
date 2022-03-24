// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestExtractConnectionSettings(t *testing.T) {
	agentWithoutCa := agentv1alpha1.Agent{
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
	}

	agentWithCa := agentWithoutCa

	assocWithoutCa := agentv1alpha1.AgentKibanaAssociation{
		Agent: &agentWithoutCa,
	}
	assocWithoutCa.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
	})

	assocWithCa := agentv1alpha1.AgentKibanaAssociation{
		Agent: &agentWithCa,
	}
	assocWithCa.SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "secret-name",
		AuthSecretKey:  "user",
		URL:            "url",
		CACertProvided: true,
		CASecretName:   "ca-secret-name",
	})

	for _, tt := range []struct {
		name                   string
		agent                  agentv1alpha1.Agent
		client                 k8s.Client
		assocType              commonv1.AssociationType
		wantConnectionSettings connectionSettings
		wantErr                bool
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
			agent:     *assocWithoutCa.Agent,
			client:    k8s.NewFakeClient(),
			assocType: commonv1.KibanaAssociationType,
			wantErr:   true,
		},
		{
			name:  "happy path without ca",
			agent: *assocWithoutCa.Agent,
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "ns",
				},
				Data: map[string][]byte{
					"user": []byte("password"),
				},
			}),
			assocType: commonv1.KibanaAssociationType,
			wantConnectionSettings: connectionSettings{
				host: "url",
				credentials: association.Credentials{
					Username: "user",
					Password: "password",
				},
			},
			wantErr: false,
		},
		{
			name:  "happy path with ca",
			agent: *assocWithCa.Agent,
			client: k8s.NewFakeClient(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: "ns",
				},
				Data: map[string][]byte{
					"user": []byte("password"),
				},
			}),
			assocType: commonv1.KibanaAssociationType,
			wantConnectionSettings: connectionSettings{
				host: "url",
				ca:   "/mnt/elastic-internal/kibana-association/ns/kibana/certs/ca.crt",
				credentials: association.Credentials{
					Username: "user",
					Password: "password",
				},
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotConnectionSettings, gotErr := extractConnectionSettings(tt.agent, tt.client, tt.assocType)

			require.Equal(t, tt.wantConnectionSettings, gotConnectionSettings)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}
