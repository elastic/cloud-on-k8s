// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func Test_getBeatKibanaRoles(t *testing.T) {
	tests := []struct {
		name    string
		args    commonv1.Associated
		want    string
		wantErr bool
	}{
		{
			name:    "not a beat",
			args:    &apmv1.ApmServer{},
			wantErr: true,
		},
		{
			name:    "<7.5 kibana_user",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "6.8.0"}},
			want:    "kibana_user",
			wantErr: false,
		},
		{
			name:    ">=7.5 kibana_admin",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "7.5.0"}},
			want:    "kibana_admin",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getBeatKibanaRoles(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("getBeatKibanaRoles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getBeatKibanaRoles() got = %v, want %v", got, tt.want)
			}
		})
	}
}
