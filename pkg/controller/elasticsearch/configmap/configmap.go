// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package configmap

import (
	"context"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// NewConfigMapWithData constructs a new config map with the given data
func NewConfigMapWithData(cm, es types.NamespacedName, data map[string]string) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Labels:    label.NewLabels(es),
		},
		Data: data,
	}
}

// ReconcileScriptsConfigMap reconciles a configmap containing scripts and related configuration used by
// init containers and readiness probe.
func ReconcileScriptsConfigMap(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) error {
	span, ctx := apm.StartSpan(ctx, "reconcile_scripts", tracing.SpanTypeApp)
	defer span.End()

	fsScript, err := initcontainer.RenderPrepareFsScript(es.DownwardNodeLabels())
	if err != nil {
		return err
	}

	preStopScript, err := nodespec.RenderPreStopHookScript(services.InternalServiceURL(es))
	if err != nil {
		return err
	}

	scriptsConfigMap := NewConfigMapWithData(
		types.NamespacedName{Namespace: es.Namespace, Name: esv1.ScriptsConfigMap(es.Name)},
		k8s.ExtractNamespacedName(&es),
		map[string]string{
			nodespec.ReadinessProbeScriptConfigKey: nodespec.ReadinessProbeScript,
			nodespec.PreStopHookScriptConfigKey:    preStopScript,
			initcontainer.PrepareFsScriptConfigKey: fsScript,
			initcontainer.SuspendScriptConfigKey:   initcontainer.SuspendScript,
			initcontainer.SuspendedHostsFile:       initcontainer.RenderSuspendConfiguration(es),
		},
	)

	return ReconcileConfigMap(ctx, c, es, scriptsConfigMap)
}
