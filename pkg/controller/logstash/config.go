// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	"hash"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
)

const (
	ConfigFileName         = "logstash.yml"
	APIKeystorePath        = volume.ConfigMountPath + "/" + APIKeystoreFileName
	APIKeystoreFileName    = "api_keystore.p12"  // #nosec G101
	APIKeystoreDefaultPass = "ch@ng3m3"          // #nosec G101
	APIKeystorePassEnv     = "API_KEYSTORE_PASS" // #nosec G101
)

func reconcileConfig(params Params, configHash hash.Hash) (*settings.CanonicalConfig, error) {
	defer tracing.Span(&params.Context)()

	cfg, err := buildConfig(params)
	if err != nil {
		return nil, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return nil, err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Logstash.Namespace,
			Name:      logstashv1alpha1.ConfigSecretName(params.Logstash.Name),
			Labels:    labels.AddCredentialsLabel(params.Logstash.GetIdentityLabels()),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Logstash); err != nil {
		return nil, err
	}

	_, _ = configHash.Write(cfgBytes)

	return cfg, nil
}

func buildConfig(params Params) (*settings.CanonicalConfig, error) {
	userProvidedCfg, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	tls := tlsConfig(params.UseTLS)

	// merge with user settings last so they take precedence
	if err := cfg.MergeWith(tls, userProvidedCfg); err != nil {
		return nil, err
	}

	if err = checkTLSConfig(cfg, params.UseTLS); err != nil {
		return nil, err
	}

	return cfg, nil
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Logstash.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Logstash.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Logstash, params.Logstash.Spec.ConfigRef, ConfigFileName)
}

func defaultConfig() *settings.CanonicalConfig {
	settingsMap := map[string]interface{}{
		// Set 'api.http.host' by default to `0.0.0.0` for readiness probe to work.
		"api.http.host": "0.0.0.0",
		// Set `config.reload.automatic` to `true` to enable pipeline reloads by default
		"config.reload.automatic": true,
	}

	return settings.MustCanonicalConfig(settingsMap)
}

func tlsConfig(useTLS bool) *settings.CanonicalConfig {
	if !useTLS {
		return nil
	}
	return settings.MustCanonicalConfig(map[string]interface{}{
		"api.ssl.enabled":           true,
		"api.ssl.keystore.path":     APIKeystorePath,
		"api.ssl.keystore.password": APIKeystoreDefaultPass,
	})
}

// checkTLSConfig ensures logstash config `api.ssl.enabled` matches the TLS setting of API service, otherwise throws error
func checkTLSConfig(cfg *settings.CanonicalConfig, useTLS bool) error {
	sslEnabled, err := cfg.String("api.ssl.enabled")
	if err != nil {
		sslEnabled = "false"
	}

	if strconv.FormatBool(useTLS) != sslEnabled {
		return fmt.Errorf("API Service `spec.services.tls.selfSignedCertificate.disabled` is set to `%t`, but logstash config `api.ssl.enabled` is set to `%s`", !useTLS, sslEnabled)
	}

	return nil
}
