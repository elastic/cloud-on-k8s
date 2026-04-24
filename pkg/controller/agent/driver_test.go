// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_isFleetServerClientAuthRequired(t *testing.T) {
	tests := []struct {
		name            string
		specClientAuth  bool
		tlsDisabled     bool
		envVar          *corev1.EnvVar
		enterpriseOff   bool
		nilChecker      bool
		wantRequired    bool
		wantWarning     bool
		wantWarningLike string
	}{
		{
			name:           "spec true, no env, TLS on: required",
			specClientAuth: true,
			wantRequired:   true,
		},
		{
			name:           "spec false, no env, TLS on: not required",
			specClientAuth: false,
			wantRequired:   false,
		},
		{
			name:            "spec true, TLS disabled: not required, warning",
			specClientAuth:  true,
			tlsDisabled:     true,
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: "TLS is disabled",
		},
		{
			name:           "spec false, TLS disabled: not required, no warning",
			specClientAuth: false,
			tlsDisabled:    true,
			wantRequired:   false,
		},
		{
			name:           "spec true, env=required, TLS on: required, no warning",
			specClientAuth: true,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "required"},
			wantRequired:   true,
		},
		{
			name:           "spec false, env=required, TLS on: required, no warning",
			specClientAuth: false,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "required"},
			wantRequired:   true,
		},
		{
			name:            "spec true, env=optional, TLS on: not required, warning",
			specClientAuth:  true,
			envVar:          &corev1.EnvVar{Name: FleetServerClientAuth, Value: "optional"},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: `FLEET_SERVER_CLIENT_AUTH is set to "optional"`,
		},
		{
			name:           "spec false, env=optional, TLS on: not required, no warning",
			specClientAuth: false,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "optional"},
			wantRequired:   false,
		},
		{
			name:            "spec true, env=none, TLS on: not required, warning",
			specClientAuth:  true,
			envVar:          &corev1.EnvVar{Name: FleetServerClientAuth, Value: "none"},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: `FLEET_SERVER_CLIENT_AUTH is set to "none"`,
		},
		{
			name:           "spec false, env=none, TLS on: not required, no warning",
			specClientAuth: false,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "none"},
			wantRequired:   false,
		},
		{
			name:           "spec false, env=required, TLS disabled: not required, no warning",
			specClientAuth: false,
			tlsDisabled:    true,
			envVar:         &corev1.EnvVar{Name: FleetServerClientAuth, Value: "required"},
			wantRequired:   false,
		},
		{
			name:           "no enterprise license: not required",
			specClientAuth: true,
			enterpriseOff:  true,
			wantRequired:   false,
		},
		{
			name:           "nil license checker: not required",
			specClientAuth: true,
			nilChecker:     true,
			wantRequired:   false,
		},
		{
			name:           "spec true, env via valueFrom, TLS on: not required, warning",
			specClientAuth: true,
			envVar: &corev1.EnvVar{
				Name: FleetServerClientAuth,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
						Key:                  "client-auth",
					},
				},
			},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: "valueFrom",
		},
		{
			name:           "spec false, env via valueFrom, TLS on: not required, warning",
			specClientAuth: false,
			envVar: &corev1.EnvVar{
				Name: FleetServerClientAuth,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "my-configmap"},
						Key:                  "client-auth",
					},
				},
			},
			wantRequired:    false,
			wantWarning:     true,
			wantWarningLike: "valueFrom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := agentv1alpha1.Agent{
				Spec: agentv1alpha1.AgentSpec{
					Deployment: &agentv1alpha1.DeploymentSpec{},
				},
			}
			agent.Spec.HTTP.TLS.Client.Authentication = tt.specClientAuth
			if tt.tlsDisabled {
				agent.Spec.HTTP.TLS.SelfSignedCertificate = &commonv1.SelfSignedCertificate{Disabled: true}
			}
			if tt.envVar != nil {
				agent.Spec.Deployment.PodTemplate.Spec.Containers = []corev1.Container{
					{Name: "agent", Env: []corev1.EnvVar{*tt.envVar}},
				}
			}

			params := Params{
				Context: context.Background(),
				Agent:   agent,
			}
			if !tt.nilChecker {
				params.LicenseChecker = commonlicense.MockLicenseChecker{EnterpriseEnabled: !tt.enterpriseOff}
			}

			gotRequired, gotWarning, err := isFleetServerClientAuthRequired(params)
			require.NoError(t, err)
			require.Equal(t, tt.wantRequired, gotRequired)
			require.Equal(t, tt.wantWarning, gotWarning != "", "unexpected warning presence: %q", gotWarning)
			if tt.wantWarningLike != "" {
				require.Contains(t, gotWarning, tt.wantWarningLike)
			}
		})
	}
}

func Test_reconcileFleetServerClientAuth(t *testing.T) {
	const (
		agentName      = "test-agent"
		agentNamespace = "test-ns"
	)
	certRotation := certificates.RotationParams{
		Validity:     24 * time.Hour,
		RotateBefore: 1 * time.Hour,
	}
	operatorClientCertSecretName := certificates.OperatorClientCertSecretName(Namer, agentName)
	trustBundleSecretName := certificates.ClientCertTrustBundleSecretName(Namer, agentName)

	newAgent := func(annotations map[string]string) agentv1alpha1.Agent {
		return agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:        agentName,
				Namespace:   agentNamespace,
				Annotations: annotations,
			},
			Spec: agentv1alpha1.AgentSpec{
				FleetServerEnabled: true,
				Deployment:         &agentv1alpha1.DeploymentSpec{},
			},
		}
	}

	tests := []struct {
		name               string
		agent              agentv1alpha1.Agent
		existingSecrets    []corev1.Secret
		clientAuthRequired bool
		fleetCerts         *certificates.CertificatesSecret
		wantAnnotation     bool
		wantOperatorCert   bool
		wantTrustBundle    bool
		wantTrustBundleCA  bool
	}{
		{
			name:               "clientAuthRequired=false, no annotation: no-op",
			agent:              newAgent(nil),
			clientAuthRequired: false,
			wantAnnotation:     false,
			wantOperatorCert:   false,
			wantTrustBundle:    false,
		},
		{
			name: "clientAuthRequired=false, annotation present: cleanup",
			agent: newAgent(map[string]string{
				annotation.ClientAuthenticationRequiredAnnotation: "true",
			}),
			existingSecrets: []corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: operatorClientCertSecretName, Namespace: agentNamespace}},
				{ObjectMeta: metav1.ObjectMeta{Name: trustBundleSecretName, Namespace: agentNamespace}},
			},
			clientAuthRequired: false,
			wantAnnotation:     false,
			wantOperatorCert:   false,
			wantTrustBundle:    false,
		},
		{
			name:               "clientAuthRequired=true, no fleet certs: creates operator cert and trust bundle",
			agent:              newAgent(nil),
			clientAuthRequired: true,
			fleetCerts:         nil,
			wantAnnotation:     true,
			wantOperatorCert:   true,
			wantTrustBundle:    true,
			wantTrustBundleCA:  false,
		},
		{
			name:               "clientAuthRequired=true, fleet certs with cert: trust bundle includes HTTP cert",
			agent:              newAgent(nil),
			clientAuthRequired: true,
			fleetCerts: &certificates.CertificatesSecret{
				Secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: []byte("fleet-server-cert-data"),
					},
				},
			},
			wantAnnotation:    true,
			wantOperatorCert:  true,
			wantTrustBundle:   true,
			wantTrustBundleCA: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the initial set of objects for the fake client.
			initObjs := slices.Clone(tt.existingSecrets)

			agent := tt.agent.DeepCopy()
			fakeClient := k8s.NewFakeClient(agent)
			// Add any pre-existing secrets.
			for i := range initObjs {
				require.NoError(t, fakeClient.Create(context.Background(), &initObjs[i]))
			}

			params := Params{
				Context: context.Background(),
				Client:  fakeClient,
				Agent:   *agent,
				OperatorParams: operator.Parameters{
					CertRotation: certRotation,
				},
			}

			results := reconcileFleetServerClientAuth(params, tt.clientAuthRequired, tt.fleetCerts, io.Discard)
			require.False(t, results.HasError(), "unexpected error in results")

			// Check annotation.
			var updatedAgent agentv1alpha1.Agent
			require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: agentNamespace,
				Name:      agentName,
			}, &updatedAgent))
			require.Equal(t, tt.wantAnnotation, annotation.HasClientAuthenticationRequired(&updatedAgent))

			// Check operator client cert secret.
			var operatorCertSecret corev1.Secret
			err := fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: agentNamespace,
				Name:      operatorClientCertSecretName,
			}, &operatorCertSecret)
			if tt.wantOperatorCert {
				require.NoError(t, err, "operator client cert secret should exist")
				require.NotEmpty(t, operatorCertSecret.Data[certificates.CertFileName], "operator client cert should have cert data")
			} else {
				require.True(t, apierrors.IsNotFound(err), "operator client cert secret should not exist")
			}

			// Check trust bundle secret.
			var trustBundleSecret corev1.Secret
			err = fakeClient.Get(context.Background(), types.NamespacedName{
				Namespace: agentNamespace,
				Name:      trustBundleSecretName,
			}, &trustBundleSecret)
			if tt.wantTrustBundle {
				require.NoError(t, err, "trust bundle secret should exist")
				bundleData := trustBundleSecret.Data[certificates.ClientCertificatesTrustBundleFileName]
				require.NotEmpty(t, bundleData, "trust bundle should have data")
				// When fleet certs have a cert, the trust bundle should include it.
				if tt.wantTrustBundleCA {
					require.Contains(t, string(bundleData), "fleet-server-cert-data", "trust bundle should contain the fleet server cert data")
				}
			} else {
				require.True(t, apierrors.IsNotFound(err), "trust bundle secret should not exist")
			}
		})
	}
}
