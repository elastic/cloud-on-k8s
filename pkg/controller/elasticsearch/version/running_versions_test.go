// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCurrentVersions(t *testing.T) {
	tests := []struct {
		name    string
		args    []corev1.Pod
		want    *version.Version
		wantErr bool
	}{
		{
			name:    "no pods",
			args:    nil,
			want:    nil,
			wantErr: false,
		},
		{
			name: "no versions in pods",
			args: []corev1.Pod{
				{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "one pod",
			args: []corev1.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							label.VersionLabelName: "1.0.0",
						},
					},
				},
			},
			want: &version.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Label: "",
			},
			wantErr: false,
		},
		{
			name: "n pods",
			args: []corev1.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							label.VersionLabelName: "2.0.0",
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							label.VersionLabelName: "1.0.0",
						},
					},
				},
			},
			want: &version.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Label: "",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MinVersion(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("MinVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MinVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
