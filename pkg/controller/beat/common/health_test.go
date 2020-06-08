// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common_test

import (
	"testing"

	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/stretchr/testify/require"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func Test_CalculateHealth(t *testing.T) {
	noAssociation := &beatv1beta1.Beat{}
	createAssociation := func(associationEstablished bool) commonv1.Association {
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
		association    commonv1.Association
		ready, desired int32
		want           beatv1beta1.BeatHealth
	}{
		{
			name:        "no association, 0 desired",
			association: noAssociation,
			ready:       0,
			desired:     0,
			want:        beatv1beta1.BeatRedHealth,
		},
		{
			name:        "no association, all ready",
			association: noAssociation,
			ready:       3,
			desired:     3,
			want:        beatv1beta1.BeatGreenHealth,
		},
		{
			name:        "no association, some ready",
			association: noAssociation,
			ready:       1,
			desired:     5,
			want:        beatv1beta1.BeatYellowHealth,
		},
		{
			name:        "no association, none ready",
			association: noAssociation,
			ready:       0,
			desired:     4,
			want:        beatv1beta1.BeatRedHealth,
		},
		{
			name:        "association not established, all ready",
			association: createAssociation(false),
			ready:       2,
			desired:     2,
			want:        beatv1beta1.BeatRedHealth,
		},
		{
			name:        "association established, 0 desired",
			association: createAssociation(true),
			want:        beatv1beta1.BeatRedHealth,
		},
		{
			name:        "association established, all ready",
			association: createAssociation(true),
			ready:       2,
			desired:     2,
			want:        beatv1beta1.BeatGreenHealth,
		},
		{
			name:        "association established, some ready",
			association: createAssociation(true),
			ready:       1,
			desired:     5,
			want:        beatv1beta1.BeatYellowHealth,
		},
		{
			name:        "association established, none ready",
			association: createAssociation(true),
			ready:       0,
			desired:     4,
			want:        beatv1beta1.BeatRedHealth,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := beatcommon.CalculateHealth(tt.association, tt.ready, tt.desired)
			require.Equal(t, tt.want, got)
		})
	}
}
