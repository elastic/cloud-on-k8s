// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/validation"
)

func Test_eulaAccepted(t *testing.T) {
	type args struct {
		ctx Context
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "No Eula set FAIL",
			args: args{},
			want: validation.Result{Allowed: false, Reason: "Please set the field eula.accepted to true to accept the EULA"},
		},
		{
			name: "Eula accepted OK",
			args: args{
				ctx: Context{
					Proposed: v1alpha1.EnterpriseLicense{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							Eula: v1alpha1.EulaState{
								Accepted: true,
							},
						},
					},
				},
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eulaAccepted(tt.args.ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("eulaAccepted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_requiredFields(t *testing.T) {
	type args struct {
		ctx Context
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "zero values FAIL",
			args: args{},
			want: validation.Result{
				Allowed: false,
				Reason:  "required fields are missing: [spec.issuer spec.issued_to spec.expiry_date_in_millis spec.start_date_in_millis spec.issue_date_in_millis spec.uid]",
			},
		},
		{
			name: "single field missing FAIL",
			args: args{
				ctx: Context{
					Proposed: v1alpha1.EnterpriseLicense{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								UID:                "some",
								IssueDateInMillis:  1,
								ExpiryDateInMillis: 1,
								IssuedTo:           "",
								Issuer:             "foo",
								StartDateInMillis:  1,
							},
						},
					},
				},
			},
			want: validation.Result{
				Allowed: false,
				Reason:  "required fields are missing: [spec.issued_to]",
			},
		},
		{
			name: "all fields present OK",
			args: args{
				ctx: Context{
					Proposed: v1alpha1.EnterpriseLicense{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								UID:                "some",
								IssueDateInMillis:  1,
								ExpiryDateInMillis: 1,
								IssuedTo:           "bar",
								Issuer:             "foo",
								StartDateInMillis:  1,
							},
						},
					},
				},
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredFields(tt.args.ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("requiredFields() = %v, want %v", got, tt.want)
			}
		})
	}
}
