// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
)

// Test_AssociationConf tests that AssociationConf reads the conf from the annotation
func Test_AssociationConf(t *testing.T) {
	kb := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb",
			Namespace: "default",
			Annotations: map[string]string{
				"association.k8s.elastic.co/es-conf": `{"authSecretName":"es-default-es-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-metrics-ca","url":"https://metrics-es-http.default.svc:9200","version":"8.0.0"}`,
			},
		},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      "es",
				Namespace: "default"},
		},
	}

	assert.Nil(t, kb.EntAssociation().AssociationConf())
	assert.NotNil(t, kb.EsAssociation().AssociationConf())
}

// Test_AssociationConfs tests that if something resets the AssocConfs map, then AssociationConf() reinitializes
// the map from the annotation.
func Test_AssociationConfs(t *testing.T) {
	// simple es without associations
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es",
			Namespace: "default",
		},
	}

	// set assoc conf even if no assoc
	assert.Equal(t, 0, len(es.AssocConfs))
	for _, association := range es.GetAssociations() {
		assocConf, err := GetAssociationConf(association)
		assert.NoError(t, err)
		association.SetAssociationConf(assocConf)
	}
	assert.Equal(t, 0, len(es.AssocConfs))

	// checks that assocConfs is nil
	for _, assoc := range es.GetAssociations() {
		assert.Nil(t, assoc.AssociationConf())
	}

	// es with associations
	esMon := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "esmon",
			Namespace: "default",
			Annotations: map[string]string{
				"association.k8s.elastic.co/es-conf-864518565": `{"authSecretName":"es-default-metrics-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-metrics-ca","url":"https://metrics-es-http.default.svc:9200","version":"8.0.0"}`,
				"association.k8s.elastic.co/es-conf-1654136115": `{"authSecretName":"es-default-logs-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-logs-ca","url":"https://logs-es-http.default.svc:9200","version":"8.0.0"}`,
			},
		},
		Spec: esv1.ElasticsearchSpec{
			Monitoring: esv1.Monitoring{
				Metrics: esv1.MetricsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "metrics",
						Namespace: "default"},
					},
				},
				Logs: esv1.LogsMonitoring{
					ElasticsearchRefs: []commonv1.ObjectSelector{{
						Name:      "logs",
						Namespace: "default"},
					},
				},
			},
		},
	}

	// set assoc conf
	assert.Equal(t, 0, len(esMon.AssocConfs))
	for _, association := range esMon.GetAssociations() {
		assocConf, err := GetAssociationConf(association)
		assert.NoError(t, err)
		association.SetAssociationConf(assocConf)
	}
	assert.Equal(t, 2, len(esMon.AssocConfs))

	// simulate the case where the assocConfs map is reset, which can happen if the resource is updated
	esMon.AssocConfs = nil

	// checks that AssociationConf are not nil when AssociationConf() is called
	assert.Equal(t, 0, len(esMon.AssocConfs))
	for _, assoc := range esMon.GetAssociations() {
		assert.NotNil(t, assoc.AssociationConf())
	}
	assert.Equal(t, 2, len(esMon.AssocConfs))
}
