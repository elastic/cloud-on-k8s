// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package operator

import (
	"testing"
)

func Test_ValidateRoles(t *testing.T) {
	type args struct {
		roles []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil: OK",
			args: args{
				roles: nil,
			},
			wantErr: false,
		},
		{
			name: "valid roles: OK",
			args: args{
				roles: []string{All, WebhookServer, NamespaceOperator, GlobalOperator},
			},
			wantErr: false,
		},
		{
			name: "invalid role: FAIL",
			args: args{
				roles: []string{GlobalOperator, "blah"},
			},
			wantErr: true,
		},
		{
			name: "invalid roles: FAIL",
			args: args{
				roles: []string{"foo", "bar"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateRoles(tt.args.roles); (err != nil) != tt.wantErr {
				t.Errorf("validateRoles() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
