// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func Test_getAPMElasticsearchRoles(t *testing.T) {
	type args struct {
		associated commonv1.Associated
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Test roles for APM Server v6.8.0",
			args: args{
				associated: &apmv1.ApmServer{
					Spec: apmv1.ApmServerSpec{Version: "6.8.0"},
				},
			},
			want: "eck_apm_user_role_v6,apm_system",
		},
		{
			name: "Test roles for APM Server v6.8.99",
			args: args{
				associated: &apmv1.ApmServer{
					Spec: apmv1.ApmServerSpec{Version: "6.8.99"},
				},
			},
			want: "eck_apm_user_role_v6,apm_system",
		},
		{
			name: "Test roles for APM Server v7.1.0",
			args: args{
				associated: &apmv1.ApmServer{
					Spec: apmv1.ApmServerSpec{Version: "7.1.0"},
				},
			},
			want: "eck_apm_user_role_v7,ingest_admin,apm_system",
		},
		{
			name: "Test roles for APM Server v7.4.99",
			args: args{
				associated: &apmv1.ApmServer{
					Spec: apmv1.ApmServerSpec{Version: "7.4.99"},
				},
			},
			want: "eck_apm_user_role_v7,ingest_admin,apm_system",
		},
		{
			name: "Test roles for APM Server v7.5.0",
			args: args{
				associated: &apmv1.ApmServer{
					Spec: apmv1.ApmServerSpec{Version: "7.5.0"},
				},
			},
			want: "eck_apm_user_role_v75,ingest_admin,apm_system",
		},
		{
			name: "Test roles for APM Server v7.7.99",
			args: args{
				associated: &apmv1.ApmServer{
					Spec: apmv1.ApmServerSpec{Version: "7.7.99"},
				},
			},
			want: "eck_apm_user_role_v75,ingest_admin,apm_system",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAPMElasticsearchRoles(tt.args.associated)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAPMElasticsearchRoles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getAPMElasticsearchRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
