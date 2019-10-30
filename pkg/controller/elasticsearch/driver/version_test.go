// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testPodWithoutVersionLabel = corev1.Pod{}
)

func Test_lowestHighestSupportedVersions_VerifySupportsExistingPods(t *testing.T) {
	newPodWithVersionLabel := func(v version.Version) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					label.VersionLabelName: v.String(),
				},
			},
		}
	}
	type fields struct {
		lowestSupportedVersion  version.Version
		highestSupportedVersion version.Version
	}
	type args struct {
		pods []corev1.Pod
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name:    "no pods",
			fields:  fields{},
			args:    args{pods: []corev1.Pod{}},
			wantErr: false,
		},
		{
			name: "pod with version label at higher bound",
			fields: fields{
				lowestSupportedVersion:  version.Version{Major: 6},
				highestSupportedVersion: version.Version{Major: 7},
			},
			args:    args{pods: []corev1.Pod{newPodWithVersionLabel(version.Version{Major: 7})}},
			wantErr: false,
		},
		{
			name: "pod with version label at lower bound",
			fields: fields{
				lowestSupportedVersion:  version.Version{Major: 6},
				highestSupportedVersion: version.Version{Major: 7},
			},
			args:    args{pods: []corev1.Pod{newPodWithVersionLabel(version.Version{Major: 6})}},
			wantErr: false,
		},
		{
			name: "pod with version label within bounds",
			fields: fields{
				lowestSupportedVersion:  version.Version{Major: 6},
				highestSupportedVersion: version.Version{Major: 7},
			},
			args:    args{pods: []corev1.Pod{newPodWithVersionLabel(version.Version{Major: 6, Minor: 4, Patch: 2})}},
			wantErr: false,
		},
		{
			name:    "pod without label",
			fields:  fields{},
			args:    args{pods: []corev1.Pod{testPodWithoutVersionLabel}},
			wantErr: true,
		},
		{
			name: "pod with too low version label",
			fields: fields{
				lowestSupportedVersion:  version.Version{Major: 6},
				highestSupportedVersion: version.Version{Major: 7},
			},
			args:    args{pods: []corev1.Pod{newPodWithVersionLabel(version.Version{Major: 5})}},
			wantErr: true,
		},
		{
			name: "pod with too high version label",
			fields: fields{
				lowestSupportedVersion:  version.Version{Major: 6},
				highestSupportedVersion: version.Version{Major: 7},
			},
			args:    args{pods: []corev1.Pod{newPodWithVersionLabel(version.Version{Major: 8})}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lh := esversion.LowestHighestSupportedVersions{
				LowestSupportedVersion:  tt.fields.lowestSupportedVersion,
				HighestSupportedVersion: tt.fields.highestSupportedVersion,
			}
			d := defaultDriver{
				DefaultDriverParameters{
					SupportedVersions: lh,
				},
			}
			if err := d.verifySupportsExistingPods(tt.args.pods); (err != nil) != tt.wantErr {
				t.Errorf("verifySupportsExistingPods() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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
			got, err := minVersion(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("minVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("minVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
