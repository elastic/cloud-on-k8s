// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"testing"

	esvolume "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

func TestNewPrepareFSInitContainer(t *testing.T) {
	type args struct {
		transportCertificatesVolume volume.SecretVolume
		clusterName                 string
		volumes                     []corev1.Volume
	}
	tests := []struct {
		name      string
		args      args
		wantMount []string
		wantErr   bool
	}{
		{
			name:      "no default data volume",
			args:      args{},
			wantMount: nil,
			wantErr:   false,
		},
		{
			name: "with default data volume",
			args: args{
				volumes: []corev1.Volume{{
					Name:         esvolume.ElasticsearchDataVolumeName,
					VolumeSource: corev1.VolumeSource{},
				}},
			},
			wantMount: []string{esvolume.ElasticsearchDataVolumeName},
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPrepareFSInitContainer(tt.args.transportCertificatesVolume, tt.args.clusterName, tt.args.volumes)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPrepareFSInitContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for _, wantedMount := range tt.wantMount {
				var found bool
				for _, vm := range got.VolumeMounts {
					if vm.Name == wantedMount {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected mount %v but was not there", wantedMount)
				}
			}

		})
	}
}
