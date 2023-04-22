// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
)

func Test_getVolumesFromAssociations(t *testing.T) {
	// Note: we use setAssocConfs to set the AssociationConfs which are normally set in the reconciliation loop.
	for _, tt := range []struct {
		name                   string
		params                 Params
		setAssocConfs          func(assocs []commonv1.Association)
		wantAssociationsLength int
	}{
		{
			name: "es refs",
			params: Params{
				Logstash: logstashv1alpha1.Logstash{
					Spec: logstashv1alpha1.LogstashSpec{
						ElasticsearchRefs: []logstashv1alpha1.ElasticsearchCluster{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "elasticsearch"},
								ClusterName:    "production",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "elasticsearch2"},
								ClusterName:    "production2",
							},
						},
					},
				},
			},
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "elasticsearch-es-ca",
				})
				assocs[1].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "elasticsearch2-es-ca",
				})
			},
			wantAssociationsLength: 2,
		},
		{
			name: "one es ref with ca, another no ca",
			params: Params{
				Logstash: logstashv1alpha1.Logstash{
					Spec: logstashv1alpha1.LogstashSpec{
						ElasticsearchRefs: []logstashv1alpha1.ElasticsearchCluster{
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "uat"},
								ClusterName:    "uat",
							},
							{
								ObjectSelector: commonv1.ObjectSelector{Name: "production"},
								ClusterName:    "production",
							},
						},
					},
				},
			},
			setAssocConfs: func(assocs []commonv1.Association) {
				assocs[0].SetAssociationConf(&commonv1.AssociationConf{
					// No CASecretName
				})
				assocs[1].SetAssociationConf(&commonv1.AssociationConf{
					CASecretName: "production-es-ca",
				})
			},
			wantAssociationsLength: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assocs := tt.params.Logstash.GetAssociations()
			tt.setAssocConfs(assocs)
			associations, err := getVolumesFromAssociations(assocs)
			require.NoError(t, err)
			require.Equal(t, tt.wantAssociationsLength, len(associations))
		})
	}
}
