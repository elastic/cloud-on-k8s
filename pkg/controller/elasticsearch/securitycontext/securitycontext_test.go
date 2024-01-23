// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package securitycontext

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

func TestFor(t *testing.T) {
	type args struct {
		ver                          version.Version
		enableReadOnlyRootFilesystem bool
	}
	tests := []struct {
		name string
		args args
		want corev1.SecurityContext
	}{
		{
			name: "elasticsearch v7 with no readOnlyRootFS doesn't set Capabilities",
			args: args{
				ver:                          semver.MustParse("7.14.2"),
				enableReadOnlyRootFilesystem: false,
			},
			want: corev1.SecurityContext{
				Capabilities:             nil,
				Privileged:               ptr.To[bool](false),
				ReadOnlyRootFilesystem:   ptr.To[bool](false),
				AllowPrivilegeEscalation: ptr.To[bool](false),
			},
		},
		{
			name: "elasticsearch v8 with no readOnlyRootFS sets Capabilities",
			args: args{
				ver:                          semver.MustParse("8.7.0"),
				enableReadOnlyRootFilesystem: false,
			},
			want: corev1.SecurityContext{
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				Privileged:               ptr.To[bool](false),
				ReadOnlyRootFilesystem:   ptr.To[bool](false),
				AllowPrivilegeEscalation: ptr.To[bool](false),
			},
		},
		{
			name: "elasticsearch v8 with readOnlyRootFS sets Capabilities",
			args: args{
				ver:                          semver.MustParse("8.7.0"),
				enableReadOnlyRootFilesystem: true,
			},
			want: corev1.SecurityContext{
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				Privileged:               ptr.To[bool](false),
				ReadOnlyRootFilesystem:   ptr.To[bool](true),
				AllowPrivilegeEscalation: ptr.To[bool](false),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := For(tt.args.ver, tt.args.enableReadOnlyRootFilesystem); !cmp.Equal(got, tt.want) {
				t.Errorf("For() = diff: %s", cmp.Diff(got, tt.want))
			}
		})
	}
}

func TestDefaultBeatSecurityContext(t *testing.T) {
	tests := []struct {
		name string
		ver  version.Version
		want *corev1.SecurityContext
	}{
		{
			name: "version 8.7 has no runAsNonRoot=true",
			ver:  version.MustParse("8.7.0"),
			want: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				Privileged:               ptr.To[bool](false),
				ReadOnlyRootFilesystem:   ptr.To[bool](true),
				AllowPrivilegeEscalation: ptr.To[bool](false),
			},
		},
		{
			name: "version 8.8.SNAPSHOT has runAsNonRoot=true",
			ver:  version.MustParse("8.8.0-SNAPSHOT"),
			want: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				Privileged:               ptr.To[bool](false),
				ReadOnlyRootFilesystem:   ptr.To[bool](true),
				RunAsNonRoot:             ptr.To[bool](true),
				AllowPrivilegeEscalation: ptr.To[bool](false),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultBeatSecurityContext(tt.ver); !cmp.Equal(got, tt.want) {
				t.Errorf("DefaultBeatSecurityContext() = diff: %s", cmp.Diff(got, tt.want))
			}
		})
	}
}
