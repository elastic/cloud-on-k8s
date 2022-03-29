// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func Test_checkBeatType(t *testing.T) {
	for _, tt := range []struct {
		name    string
		typ     string
		wantErr bool
	}{
		{
			name: "official type",
			typ:  "filebeat",
		},
		{
			name: "community type",
			typ:  "apachebeat",
		},
		{
			name:    "bad type - space",
			typ:     "file beat",
			wantErr: true,
		},
		{
			name:    "bad type - illegal characters",
			typ:     "filebeat$2",
			wantErr: true,
		},
		{
			name:    "injection",
			typ:     "filebeat,superuser",
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkBeatType(&Beat{Spec: BeatSpec{Type: tt.typ}})
			require.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSpec(t *testing.T) {
	tests := []struct {
		name    string
		beat    Beat
		wantErr bool
	}{
		{
			name: "deployment absent, dset present",
			beat: Beat{
				Spec: BeatSpec{
					DaemonSet: &DaemonSetSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "deployment present, dset absent",
			beat: Beat{
				Spec: BeatSpec{
					Deployment: &DeploymentSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			beat: Beat{
				Spec: BeatSpec{},
			},
			wantErr: true,
		},
		{
			name: "both present",
			beat: Beat{
				Spec: BeatSpec{
					Deployment: &DeploymentSpec{},
					DaemonSet:  &DaemonSetSpec{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSpec(&tc.beat)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkAssociations(t *testing.T) {
	type args struct {
		b *Beat
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &Beat{},
			},
			wantErr: false,
		},
		{
			name: "mix secret named and named refs: OK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						ElasticsearchRef: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
						KibanaRef:        commonv1.ObjectSelector{SecretName: "bli"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "secret named ref with a name: NOK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						ElasticsearchRef: commonv1.ObjectSelector{SecretName: "bla", Name: "bla"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "secret named ref with a namespace: NOK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						KibanaRef: commonv1.ObjectSelector{SecretName: "bli", Namespace: "blub"},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := checkAssociations(tt.args.b)
			if (len(errs) != 0) != tt.wantErr {
				t.Errorf("checkAssociationst() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}
