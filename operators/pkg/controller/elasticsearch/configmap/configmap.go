// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package configmap

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NewConfigMapWithData constructs a new config map with the given data
func NewConfigMapWithData(es types.NamespacedName, data map[string]string) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      es.Name,
			Namespace: es.Namespace,
			Labels:    label.NewLabels(es),
		},
		Data: data,
	}
}
