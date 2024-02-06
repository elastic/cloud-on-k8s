// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
)

func Test_calculateHealth(t *testing.T) {
	type params struct {
		esAssoc            bool
		esAssocEstablished bool
	}

	var noAssociation []commonv1.Association
	createAssociation := func(assocDef params) []commonv1.Association {
		ls := &lsv1alpha1.Logstash{
			Spec: lsv1alpha1.LogstashSpec{},
		}
		var result []commonv1.Association
		dummyConf := commonv1.AssociationConf{
			AuthSecretName: "name",
			AuthSecretKey:  "key",
			CACertProvided: true,
			CASecretName:   "name",
			URL:            "url",
		}
		cluster := lsv1alpha1.ElasticsearchCluster{
			ObjectSelector: commonv1.ObjectSelector{
				Name:      "es",
				Namespace: "a",
			},
			ClusterName: "test",
		}
		if assocDef.esAssoc {
			ls.Spec.ElasticsearchRefs = []lsv1alpha1.ElasticsearchCluster{cluster}
			esAssoc := lsv1alpha1.LogstashESAssociation{Logstash: ls}
			esAssoc.SetAssociationConf(&dummyConf)
			if assocDef.esAssocEstablished {
				_ = esAssoc.SetAssociationStatusMap(
					commonv1.ElasticsearchAssociationType,
					map[string]commonv1.AssociationStatus{"a/es": commonv1.AssociationEstablished})
			}
			result = append(result, &esAssoc)
		}
		return result
	}

	for _, tt := range []struct {
		name           string
		associations   []commonv1.Association
		ready, desired int32
		want           lsv1alpha1.LogstashHealth
	}{
		{
			name:         "no association, 0 desired",
			associations: noAssociation,
			ready:        0,
			desired:      0,
			want:         lsv1alpha1.LogstashRedHealth,
		},
		{
			name:         "no association, all ready",
			associations: noAssociation,
			ready:        3,
			desired:      3,
			want:         lsv1alpha1.LogstashGreenHealth,
		},
		{
			name:         "no association, some ready",
			associations: noAssociation,
			ready:        1,
			desired:      5,
			want:         lsv1alpha1.LogstashYellowHealth,
		},
		{
			name:         "no association, none ready",
			associations: noAssociation,
			ready:        0,
			desired:      4,
			want:         lsv1alpha1.LogstashRedHealth,
		},
		{
			name:         "association not established, all ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: false}),
			ready:        2,
			desired:      2,
			want:         lsv1alpha1.LogstashRedHealth,
		},
		{
			name:         "association established, 0 desired",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			want:         lsv1alpha1.LogstashRedHealth,
		},
		{
			name:         "association established, all ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			ready:        2,
			desired:      2,
			want:         lsv1alpha1.LogstashGreenHealth,
		},
		{
			name:         "association established, some ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			ready:        1,
			desired:      5,
			want:         lsv1alpha1.LogstashYellowHealth,
		},
		{
			name:         "association established, none ready",
			associations: createAssociation(params{esAssoc: true, esAssocEstablished: true}),
			ready:        0,
			desired:      4,
			want:         lsv1alpha1.LogstashRedHealth,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateHealth(tt.associations, tt.ready, tt.desired)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
