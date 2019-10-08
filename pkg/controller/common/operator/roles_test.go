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

func TestHasRole(t *testing.T) {
	type args struct {
		role  string
		roles []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "all roles means has global role",
			args: args{
				role:  GlobalOperator,
				roles: []string{All},
			},
			want: true,
		},
		{
			name: "all roles means has webhook role",
			args: args{
				role:  WebhookServer,
				roles: []string{All},
			},
			want: true,
		},
		{
			name: "all roles means has namespace role",
			args: args{
				role:  NamespaceOperator,
				roles: []string{All},
			},
			want: true,
		},
		{
			name: "specific role present",
			args: args{
				role:  NamespaceOperator,
				roles: []string{NamespaceOperator, GlobalOperator},
			},
			want: true,
		},
		{
			name: "specific role absent",
			args: args{
				role:  WebhookServer,
				roles: []string{GlobalOperator},
			},
			want: false,
		},
		{
			name: "no roles",
			args: args{
				role:  NamespaceOperator,
				roles: nil,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasRole(tt.args.role, tt.args.roles); got != tt.want {
				t.Errorf("HasRole() = %v, want %v", got, tt.want)
			}
		})
	}
}
