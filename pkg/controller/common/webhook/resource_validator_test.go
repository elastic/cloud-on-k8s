// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/stretchr/testify/require"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// alwaysBasicLicenseChecker reports no enterprise features (for license-denial tests).
type alwaysBasicLicenseChecker struct{}

func (alwaysBasicLicenseChecker) CurrentEnterpriseLicense(context.Context) (*license.EnterpriseLicense, error) {
	return nil, nil
}

func (alwaysBasicLicenseChecker) EnterpriseFeaturesEnabled(context.Context) (bool, error) {
	return false, nil
}

func (alwaysBasicLicenseChecker) Valid(context.Context, license.EnterpriseLicense) (bool, error) {
	return true, nil
}

func (alwaysBasicLicenseChecker) ValidOperatorLicenseKeyType(context.Context) (license.OperatorLicenseType, error) {
	return license.LicenseTypeBasic, nil
}

func testAgentElasticEnterprise(name string) *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "agent.k8s.elastic.co/v1alpha1",
			Kind:       "Agent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "elastic",
			Annotations: map[string]string{
				license.Annotation: "enterprise",
			},
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:    "8.10.0",
			Deployment: &agentv1alpha1.DeploymentSpec{},
		},
	}
}

func testAgentUnmanaged(name string) *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "unmanaged",
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:    "8.10.0",
			Deployment: &agentv1alpha1.DeploymentSpec{},
		},
	}
}

func TestResourceValidator_ValidateCreate(t *testing.T) {
	ctx := context.Background()
	managedNS := set.Make("elastic").AsSlice()

	type wantInvalidDetails struct {
		kind, group, name string
	}

	tests := []struct {
		name              string
		licenseChecker    license.Checker
		agent             *agentv1alpha1.Agent
		innerReturnsWarns bool
		wantInvalid       bool
		wantDetails       *wantInvalidDetails
	}{
		{
			name:           "license denial uses object GVK in invalid details",
			licenseChecker: alwaysBasicLicenseChecker{},
			agent:          testAgentElasticEnterprise("test-agent"),
			wantInvalid:    true,
			wantDetails: &wantInvalidDetails{
				kind:  "Agent",
				group: "agent.k8s.elastic.co",
				name:  "test-agent",
			},
		},
		{
			name:              "unmanaged namespace skips validation without warnings",
			licenseChecker:    nil,
			agent:             testAgentUnmanaged("test-agent"),
			innerReturnsWarns: true,
		},
		{
			name:              "license denial returns before inner validate suppressing warnings",
			licenseChecker:    alwaysBasicLicenseChecker{},
			agent:             testAgentElasticEnterprise("test-agent"),
			innerReturnsWarns: true,
			wantInvalid:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			validate := func(*agentv1alpha1.Agent, *agentv1alpha1.Agent) (admission.Warnings, error) {
				called = true
				if tt.innerReturnsWarns {
					return admission.Warnings{"should-not-return"}, nil
				}
				return nil, nil
			}
			v := NewResourceFuncValidator[*agentv1alpha1.Agent](tt.licenseChecker, managedNS, validate)

			warnings, err := v.ValidateCreate(ctx, tt.agent)
			require.Nil(t, warnings)
			if tt.wantInvalid {
				require.Error(t, err)
				require.True(t, apierrors.IsInvalid(err))
			} else {
				require.NoError(t, err)
			}
			require.False(t, called, "inner validate must not run when preValidate short-circuits")

			if tt.wantDetails != nil {
				var statusErr *apierrors.StatusError
				require.True(t, errors.As(err, &statusErr))
				details := statusErr.Status().Details
				require.NotNil(t, details)
				require.Equal(t, tt.wantDetails.kind, details.Kind)
				require.Equal(t, tt.wantDetails.group, details.Group)
				require.Equal(t, tt.wantDetails.name, details.Name)
			}
		})
	}
}

func TestFuncValidator_validateCreate_usesNilOldObject(t *testing.T) {
	validator := &funcValidator[*agentv1alpha1.Agent]{
		validate: func(_ *agentv1alpha1.Agent, old *agentv1alpha1.Agent) (admission.Warnings, error) {
			require.Nil(t, old)
			return nil, nil
		},
	}

	warnings, err := validator.ValidateCreate(context.Background(), &agentv1alpha1.Agent{})
	require.Nil(t, warnings)
	require.NoError(t, err)
}
