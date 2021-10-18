// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package version

import (
	"fmt"
	"reflect"
	"testing"

	semver "github.com/blang/semver/v4"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFromLabels(t *testing.T) {
	labelName := "label-with-version"
	tests := []struct {
		name    string
		labels  map[string]string
		want    Version
		wantErr bool
	}{
		{
			name:    "happy path",
			labels:  map[string]string{labelName: "7.7.0"},
			want:    semver.MustParse("7.7.0"),
			wantErr: false,
		},
		{
			name:    "label not set",
			labels:  map[string]string{},
			wantErr: true,
		},
		{
			name:    "labels nil",
			labels:  map[string]string{},
			wantErr: true,
		},
		{
			name:    "invalid version",
			labels:  map[string]string{labelName: "invalid"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromLabels(tt.labels, labelName)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func ptr(v Version) *Version {
	return &v
}

func TestMinInPods(t *testing.T) {
	type args struct {
		pods      []corev1.Pod
		labelName string
	}
	tests := []struct {
		name    string
		args    args
		want    *Version
		wantErr bool
	}{
		{
			name: "returns the min version of the list",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.1"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.0"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.1"}}},
				},
				labelName: "version-label",
			},
			want:    ptr(semver.MustParse("7.7.0")),
			wantErr: false,
		},
		{
			name: "all Pods run the same version",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.1"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.1"}}},
				},
				labelName: "version-label",
			},
			want:    ptr(semver.MustParse("7.7.1")),
			wantErr: false,
		},
		{
			name: "invalid version: error out",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "invalid"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.1"}}},
				},
				labelName: "version-label",
			},
			wantErr: true,
		},
		{
			name: "no value for the version label: error out",
			args: args{
				pods: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"version-label": "7.7.1"}}},
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"another-label": "7.7.1"}}},
				},
				labelName: "another-label",
			},
			wantErr: true,
		},
		{
			name: "empty list of Pods",
			args: args{
				pods:      nil,
				labelName: "version-label",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MinInPods(tt.args.pods, tt.args.labelName)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestMinInStatefulSets(t *testing.T) {
	ssetWithPodLabel := func(labelName string, value string) appsv1.StatefulSet {
		return appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{labelName: value}}}}}
	}

	type args struct {
		ssets     []appsv1.StatefulSet
		labelName string
	}
	tests := []struct {
		name    string
		args    args
		want    *Version
		wantErr bool
	}{
		{
			name: "returns the min version of the list",
			args: args{
				ssets: []appsv1.StatefulSet{
					ssetWithPodLabel("version-label", "7.7.1"),
					ssetWithPodLabel("version-label", "7.7.0"),
					ssetWithPodLabel("version-label", "7.7.0"),
					ssetWithPodLabel("version-label", "7.7.1"),
				},
				labelName: "version-label",
			},
			want:    ptr(semver.MustParse("7.7.0")),
			wantErr: false,
		},
		{
			name: "all StatefulSets specify the same version",
			args: args{
				ssets: []appsv1.StatefulSet{
					ssetWithPodLabel("version-label", "7.7.1"),
					ssetWithPodLabel("version-label", "7.7.1"),
				},
				labelName: "version-label",
			},
			want:    ptr(semver.MustParse("7.7.1")),
			wantErr: false,
		},
		{
			name: "invalid version: error out",
			args: args{
				ssets: []appsv1.StatefulSet{
					ssetWithPodLabel("version-label", "invalid"),
					ssetWithPodLabel("version-label", "7.7.1"),
				},
				labelName: "version-label",
			},
			wantErr: true,
		},
		{
			name: "no value for the version label: error out",
			args: args{
				ssets: []appsv1.StatefulSet{
					ssetWithPodLabel("version-label", "invalid"),
					ssetWithPodLabel("another-label", "7.7.1"),
				},
				labelName: "another-label",
			},
			wantErr: true,
		},
		{
			name: "empty list of StatefulSets",
			args: args{
				ssets:     nil,
				labelName: "version-label",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MinInStatefulSets(tt.args.ssets, tt.args.labelName)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestMinMaxVersion_WithMin(t *testing.T) {
	type fields struct {
		Min Version
		Max Version
	}
	type args struct {
		min Version
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   MinMaxVersion
	}{
		{
			name: "No minimum",
			fields: fields{
				Min: semver.MustParse("6.8.0"),
				Max: semver.MustParse("8.0.0"),
			},
			args: args{
				min: Version{},
			},
			want: MinMaxVersion{
				Min: semver.MustParse("6.8.0"),
				Max: semver.MustParse("8.0.0"),
			},
		},
		{
			name: "min >= global min",
			fields: fields{
				Min: semver.MustParse("7.10.0"),
				Max: semver.MustParse("8.0.0"),
			},
			args: args{
				min: semver.MustParse("7.10.0"),
			},
			want: MinMaxVersion{
				Min: semver.MustParse("7.10.0"),
				Max: semver.MustParse("8.0.0"),
			},
		},
		{
			name: "min < global min",
			fields: fields{
				Min: semver.MustParse("6.8.0"),
				Max: semver.MustParse("8.0.0"),
			},
			args: args{
				min: semver.MustParse("7.10.0"),
			},
			want: MinMaxVersion{
				Min: semver.MustParse("7.10.0"),
				Max: semver.MustParse("8.0.0"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mnv := MinMaxVersion{
				Min: tt.fields.Min,
				Max: tt.fields.Max,
			}
			if got := mnv.WithMin(tt.args.min); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WithMin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMinFrom(t *testing.T) {
	for _, tt := range []struct {
		name                string
		major, minor, patch uint64
	}{
		{
			name:  "8.0.0",
			major: 8,
			minor: 0,
			patch: 0,
		},
		{
			name:  "7.99.99",
			major: 7,
			minor: 99,
			patch: 99,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotVer := MinFor(tt.major, tt.minor, tt.patch)
			require.True(t, gotVer.LT(From(int(tt.major), int(tt.minor), int(tt.patch))))
			require.True(t, gotVer.EQ(MustParse(fmt.Sprintf("%d.%d.%d-1", tt.major, tt.minor, tt.patch))))
		})
	}
}
