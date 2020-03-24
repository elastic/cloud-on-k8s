// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"context"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReconcileConfigSecret reconciles the expected Kibana config secret for the given Kibana resource.
// This managed secret is mounted into each pod of the Kibana deployment.
func ReconcileConfigSecret(
	ctx context.Context,
	client k8s.Client,
	kb kbv1.Kibana,
	kbSettings CanonicalConfig,
	operatorInfo about.OperatorInfo,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_config_secret", tracing.SpanTypeApp)
	defer span.End()

	settingsYamlBytes, err := kbSettings.Render()
	if err != nil {
		return err
	}
	telemetryYamlBytes, err := getTelemetryYamlBytes(operatorInfo)
	if err != nil {
		return err
	}
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kb.Namespace,
			Name:      SecretName(kb),
			Labels: common.AddCredentialsLabel(map[string]string{
				label.KibanaNameLabelName: kb.Name,
			}),
		},
		Data: map[string][]byte{
			SettingsFilename:  settingsYamlBytes,
			telemetryFilename: telemetryYamlBytes,
		},
	}

	_, err = reconciler.ReconcileSecret(client, expected, &kb)
	return err
}
