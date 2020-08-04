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
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Constants to use for the config files in a Kibana pod.
const (
	ConfigVolumeName                   = "elastic-internal-kibana-config-local"
	ConfigVolumeMountPath              = "/usr/share/kibana/config"
	InitContainerConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config-local"

	// InternalConfigVolumeName is a volume which contains the generated configuration.
	InternalConfigVolumeName      = "elastic-internal-kibana-config"
	InternalConfigVolumeMountPath = "/mnt/elastic-internal/kibana-config"

	telemetryFilename = "telemetry.yml"
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

// ECK is a helper struct to marshal telemetry information.
type ECK struct {
	ECK about.OperatorInfo `json:"eck"`
}

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
				KibanaNameLabelName: kb.Name,
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

// getTelemetryYamlBytes returns the YAML bytes for the information on ECK.
func getTelemetryYamlBytes(operatorInfo about.OperatorInfo) ([]byte, error) {
	return yaml.Marshal(ECK{operatorInfo})
}
