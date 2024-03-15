// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package configmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func Test_NewConfigMapWithData(t *testing.T) {
	cmNsn := types.NamespacedName{Namespace: "ns1", Name: "cm-name"}
	esNsn := types.NamespacedName{Namespace: "ns1", Name: "es-name"}
	data := map[string]string{"foo": "42"}
	cm := NewConfigMapWithData(cmNsn, esNsn, data)

	assert.Equal(t, "cm-name", cm.Name)
	assert.Equal(t, "es-name", cm.Labels["elasticsearch.k8s.elastic.co/cluster-name"])
	assert.Equal(t, "elasticsearch", cm.Labels["common.k8s.elastic.co/type"])
	assert.Equal(t, "42", cm.Data["foo"])
}
