// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"path"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// setOutput will set the output section in Beat config according to the association configuration
func setOutput(cfg *settings.CanonicalConfig, client k8s.Client, associated commonv1.Association) error {
	if !associated.AssociationConf().IsConfigured() {
		return nil
	}

	username, password, err := association.ElasticsearchAuthSettings(client, associated)
	if err != nil {
		return err
	}

	esOutput := settings.MustCanonicalConfig(
		map[string]interface{}{
			"output.elasticsearch": map[string]interface{}{
				"hosts":    []string{associated.AssociationConf().GetURL()},
				"username": username,
				"password": password,
			},
		})

	if err := cfg.MergeWith(esOutput); err != nil {
		return err
	}

	if associated.AssociationConf().GetCACertProvided() {
		if err := cfg.MergeWith(settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch.ssl.certificate_authorities": path.Join(CAMountPath, CAFileName),
			})); err != nil {
			return err
		}
	}

	return nil
}

// buildBeatConfig builds Beat config based on Beat spec and default config provided. Output configuration will be added
// according to `elasticsearchRef` in the spec. No other merging is performed. Returns final config bytes.
func buildBeatConfig(
	log logr.Logger,
	client k8s.Client,
	beat beatv1beta1.Beat,
	defaultConfig *settings.CanonicalConfig,
) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	// set output if needed
	if err := setOutput(cfg, client, &beat); err != nil {
		return nil, err
	}

	// use only the default config or only the provided config - no overriding, no merging
	userConfig := beat.Spec.Config
	if userConfig == nil {
		if err := cfg.MergeWith(defaultConfig); err != nil {
			return nil, err
		}
	} else {
		userCfg, err := settings.NewCanonicalConfigFrom(userConfig.Data)
		if err != nil {
			return nil, err
		}

		if err = cfg.MergeWith(userCfg); err != nil {
			return nil, err
		}
		log.V(1).Info("Replacing preset configuration by user-provided configuration")
	}

	return cfg.Render()
}

// reconcileConfig reconciles provided config bytes in a Secret under a well known key. Secret name
// is based on Beat name and type.
func reconcileConfig(
	cfgBytes []byte,
	params DriverParams,
) error {
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

	if _, err := reconciler.ReconcileSecret(params.Client, expected, &params.Beat); err != nil {
		return err
	}

	return nil
}
