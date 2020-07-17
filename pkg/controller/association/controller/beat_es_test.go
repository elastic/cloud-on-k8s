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

func Test_getBeatRoles(t *testing.T) {

	for _, tt := range []struct {
		name    string
		assoc   commonv1.Associated
		want    string
		wantErr bool
	}{
		{
			name:    "invalid assoc",
			assoc:   &apmv1.ApmServer{},
			wantErr: true,
		},
		{
			name:    "injecting a role with official Beat should fail",
			assoc:   &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat,superuser"}},
			wantErr: true,
		},
		{
			name:    "injecting a role with community Beat should fail",
			assoc:   &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "somebeat,superuser"}},
			wantErr: true,
		},
		{
			name:    "invalid official Beat version",
			assoc:   &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "7.7.0.1a", Type: "filebeat"}},
			wantErr: true,
		},
		{
			name:  "different Community Beat version", // we are not able to validate community Beat version
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Version: "7.7.0.1a", Type: "somebeat"}},
			want:  "eck_beat_es_somebeat_role",
		},
		{
			name:  "test roles for 7.0.0 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat", Version: "7.0.0"}},
			want:  "kibana_user,ingest_admin,beats_admin,monitoring_user,eck_beat_es_filebeat_role_v70",
		},
		{
			name:  "test roles for 7.2.99 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat", Version: "7.2.99"}},
			want:  "kibana_user,ingest_admin,beats_admin,monitoring_user,eck_beat_es_filebeat_role_v70",
		},
		{
			name:  "test roles for 7.3.0 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat", Version: "7.3.0"}},
			want:  "kibana_user,ingest_admin,beats_admin,remote_monitoring_agent,eck_beat_es_filebeat_role_v73",
		},
		{
			name:  "test roles for 7.4.99 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat", Version: "7.4.99"}},
			want:  "kibana_user,ingest_admin,beats_admin,remote_monitoring_agent,eck_beat_es_filebeat_role_v73",
		},
		{
			name:  "test roles for 7.5.0 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat", Version: "7.5.0"}},
			want:  "kibana_user,ingest_admin,beats_admin,remote_monitoring_agent,eck_beat_es_filebeat_role_v75",
		},
		{
			name:  "test roles for 7.6.99 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "filebeat", Version: "7.6.99"}},
			want:  "kibana_user,ingest_admin,beats_admin,remote_monitoring_agent,eck_beat_es_filebeat_role_v75",
		},
		{
			name:  "test roles for 7.7.0 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "metricbeat", Version: "7.7.0"}},
			want:  "kibana_admin,ingest_admin,beats_admin,remote_monitoring_agent,eck_beat_es_metricbeat_role_v77",
		},
		{
			name:  "test roles for 7.99.99 official Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "metricbeat", Version: "7.99.99"}},
			want:  "kibana_admin,ingest_admin,beats_admin,remote_monitoring_agent,eck_beat_es_metricbeat_role_v77",
		},
		{
			name:  "test roles for 1.2.0 community Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "somebeat", Version: "1.2.0"}},
			want:  "eck_beat_es_somebeat_role",
		},
		{
			name:  "test roles for 3.4.0 community Beat",
			assoc: &beatv1beta1.Beat{Spec: beatv1beta1.BeatSpec{Type: "somebeat", Version: "3.4.0"}},
			want:  "eck_beat_es_somebeat_role",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getBeatRoles(tt.assoc)
			if (err != nil) != tt.wantErr {
				t.Errorf("getBeatRoles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getBeatRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
