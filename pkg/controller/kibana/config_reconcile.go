// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"

	"github.com/ghodss/yaml"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

// Constants to use for the config files in a Kibana pod.
const (
	VolumeName        = "config"
	VolumeMountPath   = "/usr/share/kibana/" + VolumeName
	telemetryFilename = "telemetry.yml"
)

// ECK is a helper struct to marshal telemetry information.
type ECK struct {
	ECK about.OperatorInfo `json:"eck"`
}

// SecretVolume returns a SecretVolume to hold the Kibana config of the given Kibana resource.
func SecretVolume(kb kbv1.Kibana) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		SecretName(kb),
		VolumeName,
		VolumeMountPath,
	)
}

// SecretName is the name of the secret that holds the Kibana config for the given Kibana resource.
func SecretName(kb kbv1.Kibana) string {
	return kb.Name + "-kb-" + VolumeName
}

// ReconcileConfigSecret reconciles the expected Kibana config secret for the given Kibana resource.
// This managed secret is mounted into each pod of the Kibana deployment.
func ReconcileConfigSecret(
	ctx context.Context,
	client k8s.Client,
	kb kbv1.Kibana,
	kbSettings CanonicalConfig,
	operatorInfo about.OperatorInfo,
	meta metadata.Metadata,
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
			Namespace:   kb.Namespace,
			Name:        SecretName(kb),
			Labels:      common.AddCredentialsLabel(maps.Clone(meta.Labels)),
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			SettingsFilename:  settingsYamlBytes,
			telemetryFilename: telemetryYamlBytes,
		},
	}

	_, err = reconciler.ReconcileSecret(client, expected, &kb)
	return err
}

// getTelemetryYamlBytes returns the YAML bytes for the information on ECK.
func getTelemetryYamlBytes(operatorInfo about.OperatorInfo) ([]byte, error) {
	return yaml.Marshal(ECK{operatorInfo})
}
