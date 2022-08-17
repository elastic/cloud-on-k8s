// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	agentFixture = func() *agentv1alpha1.Agent {
		return &agentv1alpha1.Agent{
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
	}
	assocWithCAFixture = func() agentv1alpha1.AgentKibanaAssociation {
		a := agentv1alpha1.AgentKibanaAssociation{
			Agent: agentFixture(),
		}
		a.SetAssociationConf(&commonv1.AssociationConf{
			AuthSecretName: "secret-name",
			AuthSecretKey:  "user",
			URL:            "url",
			CACertProvided: true,
			CASecretName:   "ca-secret-name",
			Version:        "8.3.0",
		})
		return a
	}
	assocWithoutCAFixture = func() agentv1alpha1.AgentKibanaAssociation {
		a := agentv1alpha1.AgentKibanaAssociation{
			Agent: agentFixture(),
		}
		a.SetAssociationConf(&commonv1.AssociationConf{
			AuthSecretName: "secret-name",
			AuthSecretKey:  "user",
			URL:            "url",
			Version:        "8.3.0",
		})
		return a
	}
)

func TestExtractConnectionSettings(t *testing.T) {
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
			agent:     *assocWithoutCAFixture().Agent,
			client:    k8s.NewFakeClient(),
			assocType: commonv1.KibanaAssociationType,
			wantErr:   true,
		},
		{
			name:  "happy path without ca",
			agent: *assocWithoutCAFixture().Agent,
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
				version: "8.3.0",
			},
			wantErr: false,
		},
		{
			name:  "happy path with ca",
			agent: *assocWithCAFixture().Agent,
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
				host:       "url",
				caFileName: "/mnt/elastic-internal/kibana-association/ns/kibana/certs/ca.crt",
				credentials: association.Credentials{
					Username: "user",
					Password: "password",
				},
				version: "8.3.0",
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotConnectionSettings, _, gotErr := extractPodConnectionSettings(context.Background(), tt.agent, tt.client, tt.assocType)

			require.Equal(t, tt.wantConnectionSettings, gotConnectionSettings)
			require.Equal(t, tt.wantErr, gotErr != nil)
		})
	}
}

func Test_extractClientConnectionSettings(t *testing.T) {
	// assoc secret all other cases are tested in TestExtractConnectionSettings
	assocSecretFixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-name",
			Namespace: "ns",
		},
		Data: map[string][]byte{
			"user": []byte("password"),
		},
	}

	// setup cert fixtures
	bytes, err := os.ReadFile(filepath.Join("testdata", "ca.crt"))
	require.NoError(t, err)
	certs, err := certificates.ParsePEMCerts(bytes)
	require.NoError(t, err)
	caSecretFixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca-secret-name",
			Namespace: "ns",
		},
		Data: map[string][]byte{
			"ca.crt": bytes,
		},
	}

	type args struct {
		agent  agentv1alpha1.Agent
		client k8s.Client
	}
	tests := []struct {
		name    string
		args    args
		want    connectionSettings
		wantErr bool
	}{
		{
			name: "agent/kibana with CA",
			args: args{
				agent:  *assocWithCAFixture().Agent,
				client: k8s.NewFakeClient(assocSecretFixture, caSecretFixture),
			},
			want: connectionSettings{
				host:       "url",
				caFileName: "/mnt/elastic-internal/kibana-association/ns/kibana/certs/ca.crt",
				version:    "8.3.0",
				credentials: association.Credentials{
					Username: "user",
					Password: "password",
				},
				caCerts: certs,
			},
			wantErr: false,
		},
		{
			name: "agent/kibana without CA",
			args: args{
				agent:  *assocWithoutCAFixture().Agent,
				client: k8s.NewFakeClient(assocSecretFixture),
			},
			want: connectionSettings{
				host: "url",
				credentials: association.Credentials{
					Username: "user",
					Password: "password",
				},
				version: "8.3.0",
			},
			wantErr: false,
		},
		{
			name: "missing certificates secret",
			args: args{
				agent:  *assocWithCAFixture().Agent,
				client: k8s.NewFakeClient(assocSecretFixture),
			},
			want:    connectionSettings{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractClientConnectionSettings(context.Background(), tt.args.agent, tt.args.client, "kibana")
			if (err != nil) != tt.wantErr {
				t.Errorf("extractClientConnectionSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractClientConnectionSettings() got = %v, want %v", got, tt.want)
			}
		})
	}
}
