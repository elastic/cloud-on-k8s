// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/filebeat"
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
			name:    "not a valid version",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "not-a-version", Type: "filebeat"}},
			wantErr: true,
		},
		{
			name:    "different Community Beat version", // we are not able to validate community Beat version
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "not-a-version", Type: "community_beat"}},
			want:    "eck_beat_kibana_community_beat_role",
			wantErr: false,
		},
		{
			name:    "<7.3 Beat",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "7.0.0", Type: string(filebeat.Type)}},
			want:    "kibana_user,ingest_admin,beats_admin,eck_beat_kibana_filebeat_role_v70",
			wantErr: false,
		},
		{
			name:    "<7.7 Beat",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "7.6.0", Type: string(filebeat.Type)}},
			want:    "kibana_user,ingest_admin,beats_admin,eck_beat_kibana_filebeat_role_v73",
			wantErr: false,
		},
		{
			name:    ">=7.8 Beat",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "7.8.0", Type: string(filebeat.Type)}},
			want:    "kibana_admin,ingest_admin,beats_admin,eck_beat_kibana_filebeat_role_v77",
			wantErr: false,
		},
		{
			name:    "community Beat",
			args:    &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "1.2.0", Type: "community_beat"}},
			want:    "eck_beat_kibana_community_beat_role",
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
