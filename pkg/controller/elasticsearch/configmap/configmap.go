// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package configmap

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// ReconcileScriptsConfigMap reconciles a configmap containing scripts used by
// init containers and readiness probe.
func ReconcileScriptsConfigMap(c k8s.Client, scheme *runtime.Scheme, es v1alpha1.Elasticsearch) error {
	fsScript, err := initcontainer.RenderPrepareFsScript()
	if err != nil {
		return err
	}

	scriptsConfigMap := NewConfigMapWithData(
		types.NamespacedName{Namespace: es.Namespace, Name: name.ScriptsConfigMap(es.Name)},
		map[string]string{
			nodespec.ReadinessProbeScriptConfigKey: nodespec.ReadinessProbeScript,
			initcontainer.PrepareFsScriptConfigKey: fsScript,
		},
	)

	return ReconcileConfigMap(c, scheme, es, scriptsConfigMap)
}
