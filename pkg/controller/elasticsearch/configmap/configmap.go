// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package configmap

import (
	"context"
	"reflect"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ReconcileScriptsConfigMap reconciles a configmap containing scripts and related configuration used by
// init containers and readiness probe. The scripts ConfigMap content feeds into the pod-template config
// hash, so any change to the rendered scripts (including label ordering) will trigger a rolling restart.
func ReconcileScriptsConfigMap(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, meta metadata.Metadata) error {
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

	nsn := types.NamespacedName{Name: esv1.ScriptsConfigMap(es.Name), Namespace: es.Namespace}
	scriptsConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nsn.Name,
			Namespace:   nsn.Namespace,
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string]string{
			nodespec.LegacyReadinessProbeScriptConfigKey: nodespec.LegacyReadinessProbeScript,
			nodespec.ReadinessPortProbeScriptConfigKey:   nodespec.ReadinessPortProbeScript,
			nodespec.PreStopHookScriptConfigKey:          preStopScript,
			initcontainer.PrepareFsScriptConfigKey:       fsScript,
			initcontainer.SuspendScriptConfigKey:         initcontainer.SuspendScript,
			initcontainer.SuspendedHostsFile:             initcontainer.RenderSuspendConfiguration(es),
		},
	}
	return reconcileConfigMap(ctx, c, es, scriptsConfigMap)
}

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func reconcileConfigMap(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	expected corev1.ConfigMap,
) error {
	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &es,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
					!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
					!reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
				reconciled.Data = expected.Data
			},
		},
	)
}
