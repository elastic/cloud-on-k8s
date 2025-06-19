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

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	kbvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// HardenedSecurityContextSupportedVersion is the version in which a hardened security context is supported.
var HardenedSecurityContextSupportedVersion = version.From(7, 10, 0)

// NewScriptsConfigMapVolume creates a new volume for the ConfigMap containing scripts used by the Kibana init container.
func NewScriptsConfigMapVolume(kbName string) volume.ConfigMapVolume {
	return volume.NewConfigMapVolumeWithMode(
		kbv1.ScriptsConfigMap(kbName),
		kbvolume.ScriptsVolumeName,
		kbvolume.ScriptsVolumeMountPath,
		0755)
}

// ReconcileScriptsConfigMap reconciles the ConfigMap containing scripts used by the Kibana elastic-internal-init container.
func ReconcileScriptsConfigMap(ctx context.Context, c k8s.Client, kb kbv1.Kibana, meta metadata.Metadata) error {
	span, ctx := apm.StartSpan(ctx, "reconcile_scripts", tracing.SpanTypeApp)
	defer span.End()

	initScript, err := renderInitScript()
	if err != nil {
		return err
	}

	nsn := types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.ScriptsConfigMap(kb.Name)}
	scriptsConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nsn.Name,
			Namespace:   kb.Namespace,
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string]string{
			KibanaInitScriptConfigKey: initScript,
		},
	}

	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &kb,
			Expected:   &scriptsConfigMap,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !reflect.DeepEqual(scriptsConfigMap.Data, reconciled.Data) ||
					!maps.IsSubset(scriptsConfigMap.Labels, reconciled.Labels) ||
					!maps.IsSubset(scriptsConfigMap.Annotations, reconciled.Annotations)
			},
			UpdateReconciled: func() {
				reconciled.Data = scriptsConfigMap.Data
				reconciled.Labels = maps.Merge(reconciled.Labels, scriptsConfigMap.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, scriptsConfigMap.Annotations)
			},
		},
	)
}
