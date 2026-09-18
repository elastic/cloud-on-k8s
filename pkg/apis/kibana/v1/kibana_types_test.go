// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
)

func TestBackgroundTasksEnabled(t *testing.T) {
	tests := []struct {
		name string
		kb   Kibana
		want bool
	}{
		{
			name: "nil backgroundTasks returns false",
			kb:   Kibana{Spec: KibanaSpec{Version: "8.17.0"}},
			want: false,
		},
		{
			name: "non-nil backgroundTasks returns true",
			kb: Kibana{Spec: KibanaSpec{
				Version:         "8.17.0",
				BackgroundTasks: &KibanaBackgroundTasks{},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.kb.BackgroundTasksEnabled())
		})
	}
}

func TestActiveRoles(t *testing.T) {
	t.Run("no backgroundTasks returns single prime role with empty name", func(t *testing.T) {
		kb := Kibana{Spec: KibanaSpec{Version: "8.17.0"}}
		roles := kb.ActiveRoles()
		require.Len(t, roles, 1)
		assert.Equal(t, "", roles[0].Name, "empty Name suppresses NODE_ROLES env var")
		assert.Equal(t, label.RolePrimeValue, roles[0].LabelValue, "LabelValue=prime so the selector label is present for upgrade-path detection")
	})

	t.Run("with backgroundTasks returns UI and BG roles", func(t *testing.T) {
		kb := Kibana{Spec: KibanaSpec{
			Version:         "8.17.0",
			BackgroundTasks: &KibanaBackgroundTasks{},
		}}
		roles := kb.ActiveRoles()
		require.Len(t, roles, 2)
		assert.Equal(t, label.UIRole, roles[0])
		assert.Equal(t, label.BackgroundTasksRole, roles[1])
	})
}

func TestGetPoolIdentityLabels(t *testing.T) {
	t.Run("no backgroundTasks: single pool always gets role=prime", func(t *testing.T) {
		kb := Kibana{Name: "test-kb", Spec: KibanaSpec{Version: "8.17.0"}}
		labels := kb.GetPoolIdentityLabels(label.UIRole)
		assert.Equal(t, label.RolePrimeValue, labels[label.RoleLabelName],
			"single-pool deployment always carries role=prime so DeploymentSelector can detect upgrade path")
	})

	t.Run("with backgroundTasks: UI gets prime, BG gets background_tasks", func(t *testing.T) {
		count := int32(1)
		kb := Kibana{Name: "test-kb", Spec: KibanaSpec{
			Version:         "8.17.0",
			BackgroundTasks: &KibanaBackgroundTasks{Count: &count},
		}}

		uiLabels := kb.GetPoolIdentityLabels(label.UIRole)
		assert.Equal(t, label.RolePrimeValue, uiLabels[label.RoleLabelName])

		bgLabels := kb.GetPoolIdentityLabels(label.BackgroundTasksRole)
		assert.Equal(t, label.RoleBackgroundTasksValue, bgLabels[label.RoleLabelName])
	})
}

func TestApmEsAssociation_AssociationConfAnnotationName(t *testing.T) {
	k := Kibana{}
	require.Equal(t, "association.k8s.elastic.co/es-conf", k.EsAssociation().AssociationConfAnnotationName())
}

// Test_AssociationConf tests that AssociationConf reads the conf from the annotation.
func Test_AssociationConf(t *testing.T) {
	kb := &Kibana{
		Name:      "kb",
		Namespace: "default",
		Annotations: map[string]string{
			"association.k8s.elastic.co/es-conf": `{"authSecretName":"es-default-es-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-metrics-ca","url":"https://metrics-es-http.default.svc:9200","version":"8.0.0"}`,
		},
		Spec: KibanaSpec{
			ElasticsearchRef: commonv1.ElasticsearchSelector{
				ObjectSelector: commonv1.ObjectSelector{
					Name:      "es",
					Namespace: "default",
				},
			},
		},
	}

	entAssocConf, err := kb.EntAssociation().AssociationConf()
	assert.NoError(t, err)
	assert.Nil(t, entAssocConf)
	esAssocConf, err := kb.EsAssociation().AssociationConf()
	assert.NoError(t, err)
	assert.NotNil(t, esAssocConf)
	assert.Equal(t, "https://metrics-es-http.default.svc:9200", esAssocConf.URL)
	eprAssocConf, err := kb.EPRAssociation().AssociationConf()
	assert.NoError(t, err)
	assert.Nil(t, eprAssocConf)
}
