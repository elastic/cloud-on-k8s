// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasRequestedLicenseLevel(t *testing.T) {
	type args struct {
		annotations map[string]string
		checker     Checker
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "no request OK",
			args: args{
				annotations: nil,
				checker:     MockLicenseChecker{},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "requesting basic on basic OK",
			args: args{
				annotations: map[string]string{
					Annotation: "basic",
				},
				checker: MockLicenseChecker{},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "requesting enterprise on basic NOK",
			args: args{
				annotations: map[string]string{
					Annotation: "enterprise",
				},
				checker: MockLicenseChecker{},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "requesting non-existing license on basic OK",
			args: args{
				annotations: map[string]string{
					Annotation: "foo",
				},
				checker: MockLicenseChecker{},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "requesting basic on enterprise OK",
			args: args{
				annotations: map[string]string{
					Annotation: "basic",
				},
				checker: MockLicenseChecker{EnterpriseEnabled: true},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "requesting enterprise on enterprise OK",
			args: args{
				annotations: map[string]string{
					Annotation: "enterprise",
				},
				checker: MockLicenseChecker{EnterpriseEnabled: true},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "requesting non-existing license on enterprise OK",
			args: args{
				annotations: map[string]string{
					Annotation: "bar",
				},
				checker: MockLicenseChecker{EnterpriseEnabled: true},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HasRequestedLicenseLevel(context.Background(), tt.args.annotations, tt.args.checker)
			if tt.wantErr != (err != nil) {
				t.Errorf("HasRequestedLicenseLevel expected err %v but was %v", tt.wantErr, err)
			}
			assert.Equalf(t, tt.want, got, "HasRequestedLicenseLevel(%v, %v)", tt.args.annotations, tt.args.checker)
		})
	}
}
