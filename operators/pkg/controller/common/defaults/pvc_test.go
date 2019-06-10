// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppendDefaultPVCs(t *testing.T) {
	foo := v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}
	bar := v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
	}

	strRef := func(s string) *string {
		return &s
	}

	type args struct {
		existing []v1.PersistentVolumeClaim
		podSpec  v1.PodSpec
		defaults []v1.PersistentVolumeClaim
	}
	tests := []struct {
		name string
		args args
		want []v1.PersistentVolumeClaim
	}{
		{
			name: "append new pvcs",
			args: args{
				existing: []v1.PersistentVolumeClaim{foo},
				defaults: []v1.PersistentVolumeClaim{bar},
			},
			want: []v1.PersistentVolumeClaim{foo, bar},
		},
		{
			name: "do not overwrite or duplicate existing",
			args: args{
				existing: []v1.PersistentVolumeClaim{foo},
				defaults: []v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
						Spec: v1.PersistentVolumeClaimSpec{
							StorageClassName: strRef("custom"),
						},
					},
				},
			},
			want: []v1.PersistentVolumeClaim{foo},
		},
		{
			name: "not add a default pvc if a non-pvc volume with the same name exists",
			args: args{
				existing: []v1.PersistentVolumeClaim{foo},
				podSpec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name:         bar.Name,
							VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
						},
					},
				},
				defaults: []v1.PersistentVolumeClaim{bar},
			},
			want: []v1.PersistentVolumeClaim{foo},
		},
		{
			name: "add a default pvc if a pvcvolume with the same name exists",
			args: args{
				existing: []v1.PersistentVolumeClaim{foo},
				podSpec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name: bar.Name,
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
							},
						},
					},
				},
				defaults: []v1.PersistentVolumeClaim{bar},
			},
			want: []v1.PersistentVolumeClaim{foo, bar},
		},
		{
			name: "add a default pvc if a non-pvc volume with a different name exists",
			args: args{
				existing: []v1.PersistentVolumeClaim{foo},
				podSpec: v1.PodSpec{
					Volumes: []v1.Volume{
						{
							Name:         "not" + bar.Name,
							VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
						},
					},
				},
				defaults: []v1.PersistentVolumeClaim{bar},
			},
			want: []v1.PersistentVolumeClaim{foo, bar},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AppendDefaultPVCs(tt.args.existing, tt.args.podSpec, tt.args.defaults...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AppendDefaultPVCs() = %v, want %v", got, tt.want)
			}
		})
	}
}
