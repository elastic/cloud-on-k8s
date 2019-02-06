// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pathutil

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPVCID(t *testing.T) {
	type args struct {
		podVolumePath string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Extract Persistent Volume Claim ID",
			args: args{
				podVolumePath: "/var/lib/kubelet/pods/cb528df9-ecab-11e8-be23-080027de035f/volumes/volumes.k8s.elastic.co~elastic-local/pvc-cc6199eb-eca0-11e8-be23-080027de035f",
			},
			want: "pvc-cc6199eb-eca0-11e8-be23-080027de035f",
		},
		{
			name: "Extract another Persistent Volume Claim ID",
			args: args{
				podVolumePath: "/var/lib/kubelet/pods/cb528df9-ecab-11e8-be23-080027de035f/volumes/volumes.k8s.elastic.co~elastic-local/pvc-cc6199eb-eca0-22222-be23-080027de035f",
			},
			want: "pvc-cc6199eb-eca0-22222-be23-080027de035f",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPVCID(tt.args.podVolumePath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildSourceDir(t *testing.T) {
	type args struct {
		mountPath string
		targetDir string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Build source dir from path",
			args: args{
				mountPath: "/volumes/path",
				targetDir: path.Join("some", "path"),
			},
			want: "/volumes/path/path",
		},
		{
			name: "Build source dir from another path",
			args: args{
				mountPath: "/mnt/elastic-local-volumes",
				targetDir: path.Join("some", "path", "that", "is", "different"),
			},
			want: "/mnt/elastic-local-volumes/different",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSourceDir(tt.args.mountPath, tt.args.targetDir)
			assert.Equal(t, tt.want, got)
		})
	}
}
