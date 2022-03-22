// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

func TestApmEsAssociation_AssociationConfAnnotationName(t *testing.T) {
	k := Kibana{}
	require.Equal(t, "association.k8s.elastic.co/es-conf", k.EsAssociation().AssociationConfAnnotationName())
}

// Test_AssociationConf tests that AssociationConf reads the conf from the annotation.
func Test_AssociationConf(t *testing.T) {
	kb := &Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb",
			Namespace: "default",
			Annotations: map[string]string{
				"association.k8s.elastic.co/es-conf": `{"authSecretName":"es-default-es-beat-es-mon-user","authSecretKey":"default-es-default-esmon-beat-es-mon-user","caCertProvided":true,"caSecretName":"es-es-monitoring-default-metrics-ca","url":"https://metrics-es-http.default.svc:9200","version":"8.0.0"}`,
			},
		},
		Spec: KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      "es",
				Namespace: "default"},
		},
	}

	entAssocConf, err := kb.EntAssociation().AssociationConf()
	assert.NoError(t, err)
	assert.Nil(t, entAssocConf)
	esAssocConf, err := kb.EsAssociation().AssociationConf()
	assert.NoError(t, err)
	assert.NotNil(t, esAssocConf)
	assert.Equal(t, "https://metrics-es-http.default.svc:9200", esAssocConf.URL)
}
