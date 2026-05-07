// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	commonwebhook "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func asJSON(obj any) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return data
}

func Test_validator_Handle(t *testing.T) {
	tests := []struct {
		name              string
		enterpriseEnabled bool
		req               admission.Request
		managedNamespaces []string
		wantAllowed       bool
		wantCode          int32
	}{
		{
			name:              "accept valid creation",
			enterpriseEnabled: false,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.4")
						policy.Namespace = "ns"
						return policy
					}()),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       true,
		},
		{
			name:              "request from unmanaged namespace is ignored",
			enterpriseEnabled: false,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.3")
						policy.Namespace = "unmanaged"
						return policy
					}()),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       true,
		},
		{
			name:              "reject invalid creation for non enterprise version floor",
			enterpriseEnabled: false,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.3")
						policy.Namespace = "ns"
						return policy
					}()),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       false,
			wantCode:          422,
		},
		{
			name:              "accept enterprise creation below non enterprise floor",
			enterpriseEnabled: true,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.3")
						policy.Namespace = "ns"
						return policy
					}()),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       true,
		},
		{
			name:              "reject invalid update",
			enterpriseEnabled: false,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				OldObject: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.4")
						policy.Namespace = "ns"
						return policy
					}()),
				},
				Object: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.3")
						policy.Namespace = "ns"
						return policy
					}()),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       false,
			wantCode:          422,
		},
		{
			name:              "delete is allowed",
			enterpriseEnabled: false,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Delete,
				OldObject: runtime.RawExtension{
					Raw: asJSON(func() any {
						policy := newPolicy("9.2.3")
						policy.Namespace = "ns"
						return policy
					}()),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       true,
		},
		{
			name:              "malformed object returns bad request",
			enterpriseEnabled: false,
			req: admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object: runtime.RawExtension{
					Raw: []byte("{invalid-json"),
				},
			}},
			managedNamespaces: []string{"ns"},
			wantAllowed:       false,
			wantCode:          400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &validator{
				licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: tt.enterpriseEnabled},
			}
			v := commonwebhook.NewResourceValidator[*autoopsv1alpha1.AutoOpsAgentPolicy](nil, tt.managedNamespaces, inner)

			wh := admission.WithValidator[*autoopsv1alpha1.AutoOpsAgentPolicy](k8s.Scheme(), v)
			got := wh.Handle(context.Background(), tt.req)
			require.Equal(t, tt.wantAllowed, got.Allowed)
			if !got.Allowed {
				require.Equal(t, tt.wantCode, got.Result.Code)
			}
		})
	}
}
