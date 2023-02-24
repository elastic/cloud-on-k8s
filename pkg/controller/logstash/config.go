// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"hash"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
)

func reconcileConfig(params Params, configHash hash.Hash) error {
	defer tracing.Span(&params.Context)()

	cfgBytes, err := buildConfig(params)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Logstash.Namespace,
			Name:      logstashv1alpha1.ConfigSecretName(params.Logstash.Name),
			Labels:    labels.AddCredentialsLabel(params.Logstash.GetIdentityLabels()),
		},
		Data: map[string][]byte{
			LogstashConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Logstash); err != nil {
		return err
	}

	_, _ = configHash.Write(cfgBytes)

	return nil
}

func buildConfig(params Params) ([]byte, error) {
	userProvidedCfg, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err != nil {
		return nil, err
	}

	// merge with user settings last so they take precedence
	if err := cfg.MergeWith(userProvidedCfg); err != nil {
		return nil, err
	}

	return cfg.Render()
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Logstash.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Logstash.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Logstash, params.Logstash.Spec.ConfigRef, LogstashConfigFileName)
}

func defaultConfig() *settings.CanonicalConfig {
	settingsMap := map[string]interface{}{
		// Set 'api.http.host' by defaut to `0.0.0.0` for readiness probe to work.
		"api.http.host": "0.0.0.0",
	}

	return settings.MustCanonicalConfig(settingsMap)
}
