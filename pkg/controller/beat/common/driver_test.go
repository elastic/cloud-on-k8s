// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/stretchr/testify/require"
)

func Test_DriverParamsValidate(t *testing.T) {
	params := func(ds *beatv1beta1.DaemonSetSpec, d *beatv1beta1.DeploymentSpec) DriverParams {
		return DriverParams{
			Beat: beatv1beta1.Beat{
				Spec: beatv1beta1.BeatSpec{
					DaemonSet:  ds,
					Deployment: d,
				},
			},
		}
	}

	for _, tt := range []struct {
		name         string
		driverParams DriverParams
		wantErr      bool
	}{
		{
			driverParams: params(nil, nil),
			wantErr:      true,
		},
		{
			driverParams: params(&beatv1beta1.DaemonSetSpec{}, &beatv1beta1.DeploymentSpec{}),
			wantErr:      true,
		},
		{
			driverParams: params(&beatv1beta1.DaemonSetSpec{}, nil),
			wantErr:      false,
		},
		{
			driverParams: params(nil, &beatv1beta1.DeploymentSpec{}),
			wantErr:      false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := validateBeatSpec(tt.driverParams.Beat.Spec) != nil
			require.True(t, tt.wantErr == gotErr)
		})
	}
}
