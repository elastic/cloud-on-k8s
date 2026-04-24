// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
)

// errorLicenseChecker implements license.Checker, returning an error from all methods.
type errorLicenseChecker struct {
	err error
}

func (e errorLicenseChecker) CurrentEnterpriseLicense(context.Context) (*license.EnterpriseLicense, error) {
	return nil, e.err
}

func (e errorLicenseChecker) EnterpriseFeaturesEnabled(context.Context) (bool, error) {
	return false, e.err
}

func (e errorLicenseChecker) Valid(context.Context, license.EnterpriseLicense) (bool, error) {
	return false, e.err
}

func (e errorLicenseChecker) ValidOperatorLicenseKeyType(context.Context) (license.OperatorLicenseType, error) {
	return "", e.err
}

func Test_validClientAuthentication(t *testing.T) {
	tests := []struct {
		name           string
		agent          *agentv1alpha1.Agent
		checker        license.Checker
		wantErrCount   int
		wantErrMessage string
	}{
		{
			name: "client auth disabled: no error",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.15.0",
					FleetServerEnabled: true,
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{
								Authentication: false,
							},
						},
					},
				},
			},
			checker:      license.MockLicenseChecker{EnterpriseEnabled: false},
			wantErrCount: 0,
		},
		{
			name: "client auth enabled, enterprise license: no error",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.15.0",
					FleetServerEnabled: true,
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{
								Authentication: true,
							},
						},
					},
				},
			},
			checker:      license.MockLicenseChecker{EnterpriseEnabled: true},
			wantErrCount: 0,
		},
		{
			name: "client auth enabled, no enterprise license: error",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.15.0",
					FleetServerEnabled: true,
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{
								Authentication: true,
							},
						},
					},
				},
			},
			checker:        license.MockLicenseChecker{EnterpriseEnabled: false},
			wantErrCount:   1,
			wantErrMessage: "client certificate authentication requires an enterprise license",
		},
		{
			name: "client auth enabled, license check error: no error (fail-open)",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.15.0",
					FleetServerEnabled: true,
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{
								Authentication: true,
							},
						},
					},
				},
			},
			checker:      errorLicenseChecker{err: errors.New("license check failed")},
			wantErrCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validClientAuthentication(context.Background(), tt.agent, tt.checker)
			require.Len(t, errs, tt.wantErrCount)
			if tt.wantErrCount > 0 {
				require.Contains(t, errs[0].Error(), tt.wantErrMessage)
			}
		})
	}
}

func Test_webhookValidator_validate(t *testing.T) {
	tests := []struct {
		name       string
		agent      *agentv1alpha1.Agent
		old        *agentv1alpha1.Agent
		checker    license.Checker
		wantErr    bool
		errMessage string
	}{
		{
			name: "valid agent without client auth",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:    "8.15.0",
					Mode:       agentv1alpha1.AgentStandaloneMode,
					Deployment: &agentv1alpha1.DeploymentSpec{},
				},
			},
			checker: license.MockLicenseChecker{EnterpriseEnabled: false},
			wantErr: false,
		},
		{
			name: "client auth enabled without enterprise license",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.15.0",
					Mode:               agentv1alpha1.AgentFleetMode,
					FleetServerEnabled: true,
					Deployment:         &agentv1alpha1.DeploymentSpec{},
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{
								Authentication: true,
							},
						},
					},
				},
			},
			checker:    license.MockLicenseChecker{EnterpriseEnabled: false},
			wantErr:    true,
			errMessage: "client certificate authentication requires an enterprise license",
		},
		{
			name: "client auth enabled with enterprise license",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.15.0",
					Mode:               agentv1alpha1.AgentFleetMode,
					FleetServerEnabled: true,
					Deployment:         &agentv1alpha1.DeploymentSpec{},
					HTTP: commonv1.HTTPConfigWithClientOptions{
						TLS: commonv1.TLSWithClientOptions{
							Client: commonv1.ClientOptions{
								Authentication: true,
							},
						},
					},
				},
			},
			checker: license.MockLicenseChecker{EnterpriseEnabled: true},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &webhookValidator{licenseChecker: tt.checker}
			_, err := v.validate(context.Background(), tt.agent, tt.old)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
