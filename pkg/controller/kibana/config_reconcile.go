// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"fmt"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// Constants to use for the config files in a Kibana pod.
const (
	ConfigVolumeName                   = "elastic-internal-kibana-config-local"
	ConfigVolumeMountPath              = "/usr/share/kibana/config"
	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config-local"

	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName      = "elastic-internal-kibana-config"
	InternalConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config"

	TelemetryFilename = "telemetry.yml"
)

var (
	// ConfigSharedVolume contains the Kibana config/ directory, it's an empty volume where the required configuration
	// is initialized by the elastic-internal-init-config init container. Its content is then shared by the init container
	// that creates the keystore and the main Kibana container.
	// This is needed in order to have in a same directory both the generated configuration and the keystore file  which
	// is created in /usr/share/kibana/config since Kibana 7.9
	ConfigSharedVolume = volume.SharedVolume{
		VolumeName:             ConfigVolumeName,
		InitContainerMountPath: InitContainerConfigVolumeMountPath,
		ContainerMountPath:     ConfigVolumeMountPath,
	}
)

// ConfigVolume returns a SecretVolume to hold the Kibana config of the given Kibana resource.
func ConfigVolume(kb kbv1.Kibana) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		SecretName(kb),
		InternalConfigVolumeName,
		InternalConfigVolumeMountPath,
	)
}

// SecretName is the name of the secret that holds the Kibana config for the given Kibana resource.
func SecretName(kb kbv1.Kibana) string {
	return kb.Name + "-kb-config"
}

// ReconcileConfigSecret reconciles the expected Kibana config secret for the given Kibana resource.
// This managed secret is mounted into each pod of the Kibana deployment.
func ReconcileConfigSecret(
	ctx context.Context,
	client k8s.Client,
	kb kbv1.Kibana,
	kbSettings CanonicalConfig,
) error {
	span, ctx := apm.StartSpan(ctx, "reconcile_config_secret", tracing.SpanTypeApp)
	defer span.End()

	settingsYamlBytes, err := kbSettings.Render()
	if err != nil {
		return err
	}

	telemetryYamlBytes, err := getTelemetryYamlBytes(client, kb)
	if err != nil {
		return err
	}

	data := map[string][]byte{
		SettingsFilename: settingsYamlBytes,
	}

	if telemetryYamlBytes != nil {
		data[TelemetryFilename] = telemetryYamlBytes
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kb.Namespace,
			Name:      SecretName(kb),
			Labels: labels.AddCredentialsLabel(map[string]string{
				kblabel.KibanaNameLabelName: kb.Name,
			}),
		},
		Data: data,
	}

	_, err = reconciler.ReconcileSecret(ctx, client, expected, &kb)
	return err
}

// getUsage returns usage map object and its YAML bytes from this Kibana configuration Secret or nil
// if the Secret or usage key doesn't exist yet.
func getTelemetryYamlBytes(client k8s.Client, kb kbv1.Kibana) ([]byte, error) {
	var secret corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: kb.Namespace, Name: SecretName(kb)}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			// this secret is just about to be created, we don't know usage yet
			return nil, nil
		}

		return nil, fmt.Errorf("unexpected error while getting usage secret: %w", err)
	}

	telemetryBytes, ok := secret.Data[TelemetryFilename]
	if !ok || telemetryBytes == nil {
		// secret is there, but telemetry not populated yet
		return nil, nil
	}

	return telemetryBytes, nil
}
