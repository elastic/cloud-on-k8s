// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
)

func Test_CalculateHealth(t *testing.T) {
	type params struct {
		esAssoc            bool
		esAssocEstablished bool
		kbAssoc            bool
		kbAssocEstablished bool
	}

	var noAssociation []commonv1.Association
	createAssociation := func(assocDef params) []commonv1.Association {
		beat := &beatv1beta1.Beat{
			Spec: beatv1beta1.BeatSpec{},
		}
		var result []commonv1.Association
		dummyConf := commonv1.AssociationConf{
			AuthSecretName: "name",
			AuthSecretKey:  "key",
			CACertProvided: true,
			CASecretName:   "name",
			URL:            "url",
		}
		if assocDef.esAssoc {
			beat.Spec.ElasticsearchRef = commonv1.ObjectSelector{
				Name:      "es",
				Namespace: "a",
			}
			esAssoc := beatv1beta1.BeatESAssociation{Beat: beat}
			esAssoc.SetAssociationConf(&dummyConf)
			if assocDef.esAssocEstablished {
				_ = esAssoc.SetAssociationStatusMap(
					commonv1.ElasticsearchAssociationType,
					map[string]commonv1.AssociationStatus{"a/es": commonv1.AssociationEstablished})
			}
			result = append(result, &esAssoc)
		}
		if assocDef.kbAssoc {
			beat.Spec.KibanaRef = commonv1.ObjectSelector{
				Name:      "kb",
				Namespace: "a",
			}
			kbAssoc := beatv1beta1.BeatKibanaAssociation{Beat: beat}
			kbAssoc.SetAssociationConf(&dummyConf)
			if assocDef.kbAssocEstablished {
				_ = kbAssoc.SetAssociationStatusMap(commonv1.KibanaAssociationType,
					map[string]commonv1.AssociationStatus{"a/kb": commonv1.AssociationEstablished})
			}
			result = append(result, &kbAssoc)
		}
		return result
	}

	for _, tt := range []struct {
		name           string
		associations   []commonv1.Association
		ready, desired int32
		want           beatv1beta1.BeatHealth
	}{
		{
			name:         "no association, 0 desired",
			associations: noAssociation,
			ready:        0,
			desired:      0,
			want:         beatv1beta1.BeatRedHealth,
		},
		{
			name:         "no association, all ready",
			associations: noAssociation,
			ready:        3,
			desired:      3,
			want:         beatv1beta1.BeatGreenHealth,
		},
		{
			name:         "no association, some ready",
			associations: noAssociation,
			ready:        1,
			desired:      5,
			want:         beatv1beta1.BeatYellowHealth,
		},
		{
			name:         "no association, none ready",
			associations: noAssociation,
			ready:        0,
			desired:      4,
			want:         beatv1beta1.BeatRedHealth,
		},
		{
			name:         "association not established, all ready",
			associations: createAssociation(params{esAssoc: true}),
			ready:        2,
			desired:      2,
			want:         beatv1beta1.BeatRedHealth,
		},
		{
			name:         "association established, 0 desired",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			want:         beatv1beta1.BeatRedHealth,
		},
		{
			name:         "association established, all ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			ready:        2,
			desired:      2,
			want:         beatv1beta1.BeatGreenHealth,
		},
		{
			name:         "association established, some ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			ready:        1,
			desired:      5,
			want:         beatv1beta1.BeatYellowHealth,
		},
		{
			name:         "association established, none ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			ready:        0,
			desired:      4,
			want:         beatv1beta1.BeatRedHealth,
		},
		{
			name: "multiple associations one established, all ready",
			associations: createAssociation(params{
				esAssoc:            true,
				esAssocEstablished: true,
				kbAssoc:            true,
				kbAssocEstablished: false,
			}),
			ready:   1,
			desired: 1,
			want:    beatv1beta1.BeatRedHealth,
		},
		{
			name: "multiple associations all established, all ready",
			associations: createAssociation(params{
				esAssoc:            true,
				esAssocEstablished: true,
				kbAssoc:            true,
				kbAssocEstablished: true,
			}),
			ready:   1,
			desired: 1,
			want:    beatv1beta1.BeatGreenHealth,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := beatcommon.CalculateHealth(tt.associations, tt.ready, tt.desired)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
