// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"context"
	"reflect"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	scriptsVolumeMountPath = "/mnt/elastic-internal/scripts"
	scriptsVolumeName      = "kibana-scripts"
)

func NewScriptsConfigMapVolume(kbName string) volume.ConfigMapVolume {
	return volume.NewConfigMapVolumeWithMode(
		kbv1.ScriptsConfigMap(kbName),
		scriptsVolumeName,
		scriptsVolumeMountPath,
		0755)
}

// NewConfigMapWithData constructs a new config map with the given data
func NewConfigMapWithData(cm, kb types.NamespacedName, data map[string]string) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Labels:    label.NewLabels(kb),
		},
		Data: data,
	}
}

// init containers and readiness probe.
func ReconcileScriptsConfigMap(ctx context.Context, c k8s.Client, kb kbv1.Kibana) error {
	span, ctx := apm.StartSpan(ctx, "reconcile_scripts", tracing.SpanTypeApp)
	defer span.End()

	fsScript, err := RenderPrepareFsScript()
	if err != nil {
		return err
	}

	scriptsConfigMap := NewConfigMapWithData(
		types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.ScriptsConfigMap(kb.Name)},
		k8s.ExtractNamespacedName(&kb),
		map[string]string{
			PrepareFsScriptConfigKey: fsScript,
		},
	)

	return ReconcileConfigMap(ctx, c, kb, scriptsConfigMap)
}

// ReconcileConfigMap checks for an existing config map and updates it or creates one if it does not exist.
func ReconcileConfigMap(
	ctx context.Context,
	c k8s.Client,
	kb kbv1.Kibana,
	expected corev1.ConfigMap,
) error {
	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &kb,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Data = expected.Data
			},
		},
	)
}
