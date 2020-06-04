// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"hash"
	"path"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// setOutput will set the output section in Beat config according to the association configuration.
func setOutput(cfg *settings.CanonicalConfig, client k8s.Client, associated commonv1.Associated) error {
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

func buildBeatConfig(
	log logr.Logger,
	client k8s.Client,
	associated commonv1.Associated,
	defaultConfig *settings.CanonicalConfig,
	userConfig *commonv1.Config,
) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	if err := setOutput(cfg, client, associated); err != nil {
		return nil, err
	}

	// use only the default config or only the provided config - no overriding, no merging
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
		log.V(1).Info("Replacing ECK-managed configuration by user-provided configuration")
	}

	return cfg.Render()
}

func reconcileConfig(
	params DriverParams,
	defaultConfig *settings.CanonicalConfig,
	configHash hash.Hash,
) error {
	cfgBytes, err := buildBeatConfig(params.Logger, params.Client, params.Associated, defaultConfig, params.Config)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Owner.GetNamespace(),
			Name:      params.Namer.ConfigSecretName(params.Type, params.Owner.GetName()),
			Labels:    common.AddCredentialsLabel(params.Labels),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Client, expected, params.Owner); err != nil {
		return err
	}

	_, _ = configHash.Write(cfgBytes)

	return nil
}
