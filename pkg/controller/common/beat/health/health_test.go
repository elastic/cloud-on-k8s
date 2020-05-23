// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package health_test

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
	"github.com/stretchr/testify/require"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func Test_CalculateHealth(t *testing.T) {
	noAssociation := &beatv1beta1.Beat{}
	createAssociated := func(associationEstablished bool) commonv1.Associated {
		result := &beatv1beta1.Beat{}
		result.SetAssociationConf(&commonv1.AssociationConf{
			AuthSecretName: "name",
			AuthSecretKey:  "key",
			CACertProvided: true,
			CASecretName:   "name",
			URL:            "url",
		})

		if associationEstablished {
			result.SetAssociationStatus(commonv1.AssociationEstablished)
		}

		return result
	}

	for _, tt := range []struct {
		name           string
		associated     commonv1.Associated
		ready, desired int32
		want           health.BeatHealth
	}{
		{
			name:       "no association, 0 desired",
			associated: noAssociation,
			ready:      0,
			desired:    0,
			want:       health.BeatGreenHealth,
		},
		{
			name:       "no association, all ready",
			associated: noAssociation,
			ready:      3,
			desired:    3,
			want:       health.BeatGreenHealth,
		},
		{
			name:       "no association, some ready",
			associated: noAssociation,
			ready:      1,
			desired:    5,
			want:       health.BeatYellowHealth,
		},
		{
			name:       "no association, none ready",
			associated: noAssociation,
			ready:      0,
			desired:    4,
			want:       health.BeatRedHealth,
		},
		{
			name:       "association not established, all ready",
			associated: createAssociated(false),
			ready:      2,
			desired:    2,
			want:       health.BeatRedHealth,
		},
		{
			name:       "association established, 0 desired",
			associated: createAssociated(true),
			want:       health.BeatGreenHealth,
		},
		{
			name:       "association established, all ready",
			associated: createAssociated(true),
			ready:      2,
			desired:    2,
			want:       health.BeatGreenHealth,
		},
		{
			name:       "association established, some ready",
			associated: createAssociated(true),
			ready:      1,
			desired:    5,
			want:       health.BeatYellowHealth,
		},
		{
			name:       "association established, none ready",
			associated: createAssociated(true),
			ready:      0,
			desired:    4,
			want:       health.BeatRedHealth,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := health.CalculateHealth(tt.associated, tt.ready, tt.desired)
			require.Equal(t, tt.want, got)
		})
	}
}
