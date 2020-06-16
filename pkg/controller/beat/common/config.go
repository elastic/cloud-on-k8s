// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ConfigRefWatchName returns the name of the watch registered on Kubernetes Secret referenced in `configRef`
func ConfigRefWatchName(beat types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-configref", beat.Namespace, beat.Name)
}

// buildOutputConfig will create the output section in Beat config according to the association configuration.
func buildOutputConfig(client k8s.Client, associated beatv1beta1.BeatESAssociation) (*settings.CanonicalConfig, error) {
	if !associated.AssociationConf().IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	username, password, err := association.ElasticsearchAuthSettings(client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	esOutput := map[string]interface{}{
		"output.elasticsearch": map[string]interface{}{
			"hosts":    []string{associated.AssociationConf().GetURL()},
			"username": username,
			"password": password,
		},
	}

	if associated.AssociationConf().GetCACertProvided() {
		esOutput["output.elasticsearch.ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}

	return settings.NewCanonicalConfigFrom(esOutput)
}

// BuildKibanaConfig builds on optional Kibana configuration for dashboard setup and visualizations.
func BuildKibanaConfig(client k8s.Client, associated beatv1beta1.BeatKibanaAssociation) (*settings.CanonicalConfig, error) {
	if !associated.AssociationConf().IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	username, password, err := association.ElasticsearchAuthSettings(client, &associated)
	if err != nil {
		return settings.NewCanonicalConfig(), err
	}

	kibanaCfg := map[string]interface{}{
		"setup.dashboards.enabled": true,
		"setup.kibana": map[string]interface{}{
			"host":     associated.AssociationConf().GetURL(),
			"username": username,
			"password": password,
		},
	}

	if associated.AssociationConf().GetCACertProvided() {
		kibanaCfg["setup.kibana.ssl.certificate_authorities"] = []string{path.Join(certificatesDir(&associated), CAFileName)}
	}
	return settings.NewCanonicalConfigFrom(kibanaCfg)
}

func buildBeatConfig(
	params DriverParams,
	managedConfig *settings.CanonicalConfig,
) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	outputCfg, err := buildOutputConfig(params.Client, beatv1beta1.BeatESAssociation{Beat: &params.Beat})
	if err != nil {
		return nil, err
	}
	err = cfg.MergeWith(outputCfg, managedConfig)
	if err != nil {
		return nil, err
	}

	// get user config from `config` or `configRef`
	userConfig, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	if userConfig == nil {
		return cfg.Render()
	}

	if err = cfg.MergeWith(userConfig); err != nil {
		return nil, err
	}

	return cfg.Render()
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params DriverParams) (*settings.CanonicalConfig, error) {
	if params.Beat.Spec.Config != nil {
		userCfg, err := settings.NewCanonicalConfigFrom(params.Beat.Spec.Config.Data)
		if err != nil {
			return nil, err
		}
		return userCfg, nil
	}

	if params.Beat.Spec.ConfigRef == nil {
		return nil, nil
	}

	secret := &corev1.Secret{}
	if err := params.Client.Get(
		types.NamespacedName{
			Name:      params.Beat.Spec.ConfigRef.SecretName,
			Namespace: params.Beat.Namespace,
		}, secret); err != nil {
		return nil, err
	}

	nsn := k8s.ExtractNamespacedName(&params.Beat)
	if err := watches.WatchUserProvidedSecrets(nsn, params.DynamicWatches(), ConfigRefWatchName(nsn), []string{secret.Name}); err != nil {
		return nil, err
	}

	data, exists := secret.Data[ConfigFileName]
	if !exists {
		msg := fmt.Sprintf("no key %s in secret %s in namespace %s", ConfigFileName, secret.Name, params.Beat.Namespace)
		params.Recorder().Event(&params.Beat, corev1.EventTypeWarning, events.EventReasonUnexpected, msg)

		// create new msg to avoid duplicating secret name and namespace
		msg = fmt.Sprintf("no %s key in secret", ConfigFileName)
		params.Logger.Error(nil, msg, "namespace", params.Beat.Namespace, "beat_name", params.Beat.Name, "secret_name", secret.Name)
		return nil, fmt.Errorf(msg)
	}
	parsed, err := settings.ParseConfig(data)
	if err != nil {
		msg := fmt.Sprintf("unable to parse configuration from key %s in secret %s in namespace %s", ConfigFileName, secret.Name, params.Beat.Namespace)
		params.Recorder().Event(&params.Beat, corev1.EventTypeWarning, events.EventReasonUnexpected, msg)

		// create new msg to avoid duplicating secret name and namespace
		msg = "unable to parse configuration from secret"
		params.Logger.Error(err, msg, "namespace", params.Beat.Namespace, "beat_name", params.Beat.Name, "secret_name", secret.Name)
		return nil, err
	}
	return parsed, nil
}

func reconcileConfig(
	params DriverParams,
	managedConfig *settings.CanonicalConfig,
	configHash hash.Hash,
) error {
	cfgBytes, err := buildBeatConfig(params, managedConfig)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Beat.Namespace,
			Name:      ConfigSecretName(params.Beat.Spec.Type, params.Beat.Name),
			Labels:    common.AddCredentialsLabel(NewLabels(params.Beat)),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Client, expected, &params.Beat); err != nil {
		return err
	}

	_, _ = configHash.Write(cfgBytes)

	return nil
}
