// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	"hash"
	"regexp"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"
)

const (
	ConfigFileName         = "logstash.yml"
	APIKeystorePath        = volume.ConfigMountPath + "/" + APIKeystoreFileName
	APIKeystoreFileName    = "api_keystore.p12"  // #nosec G101
	APIKeystoreDefaultPass = "changeit"          // #nosec G101
	APIKeystorePassEnv     = "API_KEYSTORE_PASS" // #nosec G101
)

func reconcileConfig(params Params, svcUseTLS bool, configHash hash.Hash) (*settings.CanonicalConfig, configs.APIServer, error) {
	defer tracing.Span(&params.Context)()

	cfg, err := buildConfig(params, svcUseTLS)
	if err != nil {
		return nil, configs.APIServer{}, err
	}

	apiServerConfig, err := resolveAPIServerConfig(cfg, params)
	if err != nil {
		return nil, configs.APIServer{}, err
	}

	if err = checkTLSConfig(apiServerConfig, svcUseTLS); err != nil {
		return nil, configs.APIServer{}, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return nil, configs.APIServer{}, err
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

	// store the keystore password for initConfigContainer to reference,
	// so that the password does not expose in plain text
	if apiServerConfig.UseTLS() {
		expected.Data[APIKeystorePassEnv] = []byte(apiServerConfig.KeystorePassword)
	}

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Logstash); err != nil {
		return nil, configs.APIServer{}, err
	}

	_, _ = configHash.Write(cfgBytes)

	return cfg, apiServerConfig, nil
}

func buildConfig(params Params, useTLS bool) (*settings.CanonicalConfig, error) {
	userProvidedCfg, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	tls := tlsConfig(useTLS)

	// merge with user settings last so they take precedence
	if err := cfg.MergeWith(tls, userProvidedCfg); err != nil {
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

// checkTLSConfig ensures logstash config `api.ssl.enabled` matches the TLS setting of API service
// we allow disabling TLS in service and leaving `api.ssl.enabled` unset in logstash.yml, otherwise throw error
func checkTLSConfig(config configs.APIServer, useTLS bool) error {
	svcUseTLS := strconv.FormatBool(useTLS)
	sslEnabled := config.SSLEnabled
	if (svcUseTLS == sslEnabled) || (!useTLS && sslEnabled == "") {
		return nil
	}

	return fmt.Errorf("API Service `spec.services.tls.selfSignedCertificate.disabled` is set to `%t`, but logstash config `api.ssl.enabled` is set to `%s`", !useTLS, sslEnabled)
}

// resolveAPIServerConfig gives ExpectedAPIServer with the resolved ${VAR} value
func resolveAPIServerConfig(cfg *settings.CanonicalConfig, params Params) (configs.APIServer, error) {
	config := baseAPIServer(cfg)

	if unresolvedConfig := getUnresolvedVars(&config); len(unresolvedConfig) > 0 {
		combinedMap, err := getKeystoreEnvKeyValues(params)
		if err != nil {
			return configs.APIServer{}, err
		}

		patchConfigValue(unresolvedConfig, combinedMap)
	}

	return config, nil
}

// baseAPIServer gives api.* configs
func baseAPIServer(cfg *settings.CanonicalConfig) configs.APIServer {
	enabled, _ := cfg.String("api.ssl.enabled")
	keystorePassword, _ := cfg.String("api.ssl.keystore.password")
	authType, _ := cfg.String("api.auth.type")
	username, _ := cfg.String("api.auth.basic.username")
	pw, _ := cfg.String("api.auth.basic.password")

	return configs.APIServer{
		SSLEnabled:       enabled,
		KeystorePassword: keystorePassword,
		AuthType:         authType,
		Username:         username,
		Password:         pw,
	}
}

const (
	varPattern = `^\${([a-zA-Z0-9_]+)}$`
)

var (
	varRegex = regexp.MustCompile(varPattern)
)

// getUnresolvedVars matches pattern ${VAR} against the configuration value in logstash.yml, such as api.auth.basic.username:  ${API_USERNAME}
// and gives a map of string and string pointer, for example: {"API_USERNAME" : &config.Username}
// The keys in the map represent variable names that require further resolution, retrieving the values from either the Keystore or Environment variables.
// The variable name can consist of digit, underscores and letters.
func getUnresolvedVars(config *configs.APIServer) map[string]*string {
	data := make(map[string]*string)

	for _, configVal := range []*string{&config.SSLEnabled, &config.KeystorePassword, &config.AuthType, &config.Username, &config.Password} {
		if match := varRegex.FindStringSubmatch(*configVal); match != nil {
			varName := match[1]
			data[varName] = configVal
		}
	}

	return data
}

// getKeystoreEnvKeyValues gives a map that consolidate all key value pairs from user defined environment variables
// and Keystore from SecureSettings. If the same key defined in both places, keystore takes the precedence.
func getKeystoreEnvKeyValues(params Params) (map[string]string, error) {
	data := make(map[string]string)

	// from ENV
	if c := pod.ContainerByName(params.Logstash.Spec.PodTemplate.Spec, logstashv1alpha1.LogstashContainerName); c != nil {
		if err := getEnvKeyValues(params, c, data); err != nil {
			return nil, err
		}
	}

	// from keystore SecureSettings
	for _, ss := range params.Logstash.SecureSettings() {
		secret := corev1.Secret{}
		nsn := types.NamespacedName{Name: ss.SecretName, Namespace: params.Logstash.Namespace}
		if err := params.Client.Get(params.Context, nsn, &secret); err != nil {
			return nil, err
		}

		for key, value := range secret.Data {
			data[key] = string(value)
		}
	}

	return data, nil
}

func getEnvKeyValues(params Params, c *corev1.Container, data map[string]string) error {
	for _, env := range c.Env {
		data[env.Name] = env.Value
	}

	for _, envFrom := range c.EnvFrom {
		// from ConfigMap
		if envFrom.ConfigMapRef != nil {
			configMap := corev1.ConfigMap{}
			nsn := types.NamespacedName{Name: envFrom.ConfigMapRef.LocalObjectReference.Name, Namespace: params.Logstash.Namespace}
			if err := params.Client.Get(params.Context, nsn, &configMap); err != nil {
				return err
			}

			for key, value := range configMap.Data {
				data[key] = value
			}
		}

		// from Secret
		if envFrom.SecretRef != nil {
			secret := corev1.Secret{}
			nsn := types.NamespacedName{Name: envFrom.SecretRef.LocalObjectReference.Name, Namespace: params.Logstash.Namespace}
			if err := params.Client.Get(params.Context, nsn, &secret); err != nil {
				return err
			}

			for key, value := range secret.Data {
				data[key] = string(value)
			}
		}
	}

	return nil
}

// patchConfigValue resolves values in `unresolved` using `combinedMap` as a dictionary.
func patchConfigValue(unresolved map[string]*string, combinedMap map[string]string) {
	for varName, config := range unresolved {
		if actualValue, ok := combinedMap[varName]; ok {
			*config = actualValue
		}
	}
}
