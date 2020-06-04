// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package configmap

import (
	"context"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ReconcileScriptsConfigMap reconciles a configmap containing scripts used by init containers and readiness probe.
func ReconcileScriptsConfigMap(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, meta metadata.Metadata) error {
	span, _ := apm.StartSpan(ctx, "reconcile_scripts", tracing.SpanTypeApp)
	defer span.End()

	fsScript, err := initcontainer.RenderPrepareFsScript()
	if err != nil {
		return err
	}

	nsn := types.NamespacedName{Name: esv1.ScriptsConfigMap(es.Name), Namespace: es.Namespace}

	scriptsConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nsn.Name,
			Namespace:   nsn.Namespace,
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string]string{
			nodespec.ReadinessProbeScriptConfigKey: nodespec.ReadinessProbeScript,
			nodespec.PreStopHookScriptConfigKey:    nodespec.PreStopHookScript,
			initcontainer.PrepareFsScriptConfigKey: fsScript,
		},
	}

	return ReconcileConfigMap(c, es, scriptsConfigMap)
}
