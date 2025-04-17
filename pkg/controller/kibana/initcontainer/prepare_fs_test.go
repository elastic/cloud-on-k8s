// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/go-cmp/cmp"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
)

func TestNewInitContainer(t *testing.T) {
	defaultKibana := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test-ns",
		},
		Spec: kbv1.KibanaSpec{
			Version: "7.10.0",
		},
	}
	olderKibana := defaultKibana
	olderKibana.Spec.Version = "7.8.0"
	type args struct {
		kb                        kbv1.Kibana
		setDefaultSecurityContext bool
	}
	tests := []struct {
		name    string
		args    args
		want    corev1.Container
		wantErr bool
	}{
		{
			name: "newer Kibana without default security context includes plugins volume",
			args: args{
				kb:                        defaultKibana,
				setDefaultSecurityContext: false,
			},
			want: corev1.Container{
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elastic-internal-init",
				Env:             defaults.PodDownwardEnvVars(),
				Command:         []string{"/usr/bin/env", "bash", "-c", "/mnt/elastic-internal/scripts/init.sh"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "elastic-internal-kibana-config-local",
						MountPath: "/mnt/elastic-internal/kibana-config-local",
					},
					{
						Name:      "elastic-internal-kibana-config",
						MountPath: "/mnt/elastic-internal/kibana-config",
						ReadOnly:  true,
					},
					{
						Name:      "kibana-plugins",
						MountPath: "/mnt/elastic-internal/kibana-plugins-local",
					},
				},
				Resources: defaultResources,
			},
			wantErr: false,
		},
		{
			name: "newer Kibana with default security context includes plugins volume",
			args: args{
				kb:                        defaultKibana,
				setDefaultSecurityContext: true,
			},
			want: corev1.Container{
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elastic-internal-init",
				Env:             defaults.PodDownwardEnvVars(),
				Command:         []string{"/usr/bin/env", "bash", "-c", "/mnt/elastic-internal/scripts/init.sh"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "elastic-internal-kibana-config-local",
						MountPath: "/mnt/elastic-internal/kibana-config-local",
						ReadOnly:  false,
					},
					{
						Name:      "elastic-internal-kibana-config",
						MountPath: "/mnt/elastic-internal/kibana-config",
						ReadOnly:  true,
					},
					{
						Name:      "kibana-plugins",
						MountPath: "/mnt/elastic-internal/kibana-plugins-local",
						ReadOnly:  false,
					},
				},
				Resources: defaultResources,
			},
			wantErr: false,
		},
		{
			name: "older Kibana without default security context includes plugins volume",
			args: args{
				kb:                        olderKibana,
				setDefaultSecurityContext: false,
			},
			want: corev1.Container{
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elastic-internal-init",
				Env:             defaults.PodDownwardEnvVars(),
				Command:         []string{"/usr/bin/env", "bash", "-c", "/mnt/elastic-internal/scripts/init.sh"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "elastic-internal-kibana-config-local",
						MountPath: "/mnt/elastic-internal/kibana-config-local",
						ReadOnly:  false,
					},
					{
						Name:      "elastic-internal-kibana-config",
						MountPath: "/mnt/elastic-internal/kibana-config",
						ReadOnly:  true,
					},
					{
						Name:      "kibana-plugins",
						MountPath: "/mnt/elastic-internal/kibana-plugins-local",
					},
				},
				Resources: defaultResources,
			},
			wantErr: false,
		},
		{
			name: "older Kibana with default security context does not include plugins volume",
			args: args{
				kb:                        olderKibana,
				setDefaultSecurityContext: true,
			},
			want: corev1.Container{
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elastic-internal-init",
				Env:             defaults.PodDownwardEnvVars(),
				Command:         []string{"/usr/bin/env", "bash", "-c", "/mnt/elastic-internal/scripts/init.sh"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "elastic-internal-kibana-config-local",
						MountPath: "/mnt/elastic-internal/kibana-config-local",
						ReadOnly:  false,
					},
					{
						Name:      "elastic-internal-kibana-config",
						MountPath: "/mnt/elastic-internal/kibana-config",
						ReadOnly:  true,
					},
					{
						Name:      "kibana-plugins",
						MountPath: "/mnt/elastic-internal/kibana-plugins-local",
					},
				},
				Resources: defaultResources,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewInitContainer(tt.args.kb, tt.args.setDefaultSecurityContext)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewInitContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want) {
				t.Errorf("NewInitContainer() diff = %s", cmp.Diff(got, tt.want))
			}
		})
	}
}
