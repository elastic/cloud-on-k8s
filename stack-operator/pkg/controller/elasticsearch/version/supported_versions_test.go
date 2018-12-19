package version

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testPodWithoutVersionLabel = corev1.Pod{}
)

func Test_lowestHighestSupportedVersions_VerifySupportsExistingPods(t *testing.T) {
	newPodWithVersionLabel := func(v version.Version) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					ElasticsearchVersionLabelName: v.String(),
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
			lh := LowestHighestSupportedVersions{
				LowestSupportedVersion:  tt.fields.lowestSupportedVersion,
				HighestSupportedVersion: tt.fields.highestSupportedVersion,
			}
			if err := lh.VerifySupportsExistingPods(tt.args.pods); (err != nil) != tt.wantErr {
				t.Errorf("LowestHighestSupportedVersions.VerifySupportsExistingPods() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
