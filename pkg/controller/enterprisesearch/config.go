// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	ESCertsPath              = "/mnt/elastic-internal/es-certs"
	ConfigMountPath          = "/usr/share/enterprise-search/config/enterprise-search.yml"
	ConfigFilename           = "enterprise-search.yml"
	ReadinessProbeMountPath  = "/mnt/elastic-internal/scripts/readiness-probe.sh"
	ReadinessProbeFilename   = "readiness-probe.sh"
	ReadinessProbeTimeoutSec = 5

	SecretSessionSetting  = "secret_session_key"
	EncryptionKeysSetting = "secret_management.encryption_keys"
)

func ConfigSecretVolume(ent entv1beta1.EnterpriseSearch) volume.SecretVolume {
	return volume.NewSecretVolume(name.Config(ent.Name), "config", ConfigMountPath, ConfigFilename, 0444)
}

func ReadinessProbeSecretVolume(ent entv1beta1.EnterpriseSearch) volume.SecretVolume {
	// reuse the config secret
	return volume.NewSecretVolume(name.Config(ent.Name), "readiness-probe", ReadinessProbeMountPath, ReadinessProbeFilename, 0444)
}

// Reconcile reconciles the configuration of Enterprise Search: it generates the right configuration and
// stores it in a secret that is kept up to date.
// The secret contains 2 entries:
// - the Enterprise Search configuration file
// - a bash script used as readiness probe
func ReconcileConfig(driver driver.Interface, ent entv1beta1.EnterpriseSearch) (corev1.Secret, error) {
	cfg, err := newConfig(driver, ent)
	if err != nil {
		return corev1.Secret{}, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return corev1.Secret{}, err
	}

	readinessProbeBytes, err := readinessProbeScript(ent, cfg)
	if err != nil {
		return corev1.Secret{}, err
	}

	// Reconcile the configuration in a secret
	expectedConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ent.Namespace,
			Name:      name.Config(ent.Name),
			Labels:    common.AddCredentialsLabel(Labels(ent.Name)),
		},
		Data: map[string][]byte{
			ConfigFilename:         cfgBytes,
			ReadinessProbeFilename: readinessProbeBytes,
		},
	}

	return reconciler.ReconcileSecret(driver.K8sClient(), expectedConfigSecret, &ent)
}

// partialConfigWithESAuth helps parsing the configuration file to retrieve ES credentials.
type partialConfigWithESAuth struct {
	Elasticsearch struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"elasticsearch"`
}

// readinessProbeScript returns a bash script that requests the health endpoint.
func readinessProbeScript(ent entv1beta1.EnterpriseSearch, config *settings.CanonicalConfig) ([]byte, error) {
	url := fmt.Sprintf("%s://127.0.0.1:%d/api/ent/v1/internal/health", ent.Spec.HTTP.Protocol(), HTTPPort)

	// retrieve Elasticsearch user credentials from the aggregated config since it could be user-provided
	var esAuth partialConfigWithESAuth
	if err := config.Unpack(&esAuth); err != nil {
		return nil, err
	}
	basicAuthArgs := "" // no credentials: no basic auth
	if esAuth.Elasticsearch.Username != "" {
		basicAuthArgs = fmt.Sprintf("-u %s:%s", esAuth.Elasticsearch.Username, esAuth.Elasticsearch.Password)
	}

	return []byte(`#!/usr/bin/env bash

	# fail should be called as a last resort to help the user to understand why the probe failed
	function fail {
	  timestamp=$(date --iso-8601=seconds)
	  echo "{\"timestamp\": \"${timestamp}\", \"message\": \"readiness probe failed\", "$1"}" | tee /proc/1/fd/2 2> /dev/null
	  exit 1
	}

	# request timeout can be overridden from an environment variable
	READINESS_PROBE_TIMEOUT=${READINESS_PROBE_TIMEOUT:=` + fmt.Sprintf("%d", ReadinessProbeTimeoutSec) + `}

	# request the health endpoint and expect http status code 200
	status=$(curl -o /dev/null -w "%{http_code}" ` + url + ` ` + basicAuthArgs + ` -k -s --max-time ${READINESS_PROBE_TIMEOUT})
	curl_rc=$?

	if [[ ${curl_rc} -ne 0 ]]; then
	  fail "\"curl_rc\": \"${curl_rc}\""
	fi

	if [[ ${status} == "200" ]]; then
	  exit 0
	else
	  fail " \"status\": \"${status}\", \"version\":\"${version}\" "
	fi
`), nil
}

// newConfig builds a single merged config from:
// - ECK-managed default configuration
// - association configuration (eg. ES credentials)
// - TLS settings configuration
// - user-provided plaintext configuration
// - user-provided secret configuration
// In case of duplicate settings, the last one takes precedence.
func newConfig(driver driver.Interface, ent entv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	reusedCfg, err := getOrCreateReusableSettings(driver.K8sClient(), ent)
	if err != nil {
		return nil, err
	}
	tlsCfg := tlsConfig(ent)
	associationCfg, err := associationConfig(driver.K8sClient(), ent)
	if err != nil {
		return nil, err
	}
	specConfig := ent.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}
	userProvidedCfg, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return nil, err
	}
	userProvidedSecretCfg, err := parseConfigRef(driver, ent)
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig(ent)

	// merge with user settings last so they take precedence
	err = cfg.MergeWith(reusedCfg, tlsCfg, associationCfg, userProvidedCfg, userProvidedSecretCfg)
	return cfg, err
}

// reusableSettings captures secrets settings in the Enterprise Search configuration that we want to reuse.
type reusableSettings struct {
	SecretSession  string   `config:"secret_session_key"`
	EncryptionKeys []string `config:"secret_management.encryption_keys"`
}

// getOrCreateReusableSettings reads the current configuration and reuse existing secrets it they exist.
func getOrCreateReusableSettings(c k8s.Client, ent entv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	cfg, err := getExistingConfig(c, ent)
	if err != nil {
		return nil, err
	}

	var e reusableSettings
	if cfg == nil {
		e = reusableSettings{}
	} else if err := cfg.Unpack(&e); err != nil {
		return nil, err
	}
	if len(e.SecretSession) == 0 {
		e.SecretSession = string(common.RandomBytes(32))
	}
	if len(e.EncryptionKeys) == 0 {
		e.EncryptionKeys = []string{string(common.RandomBytes(32))}
	}
	return settings.MustCanonicalConfig(e), nil
}

// getExistingConfig retrieves the canonical config, if one exists
func getExistingConfig(client k8s.Client, ent entv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: ent.Namespace,
		Name:      name.Config(ent.Name),
	}
	err := client.Get(key, &secret)
	if err != nil && apierrors.IsNotFound(err) {
		log.V(1).Info("Enterprise Search config secret does not exist", "namespace", ent.Namespace, "ent_name", ent.Name)
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	rawCfg, exists := secret.Data[ConfigFilename]
	if !exists {
		return nil, nil
	}
	cfg, err := settings.ParseConfig(rawCfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseConfigRef(driver driver.Interface, ent entv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	return common.ParseConfigRef(driver, &ent, ent.Spec.ConfigRef, ConfigFilename)
}

func defaultConfig(ent entv1beta1.EnterpriseSearch) *settings.CanonicalConfig {
	return settings.MustCanonicalConfig(map[string]interface{}{
		"ent_search.external_url":        fmt.Sprintf("%s://localhost:%d", ent.Spec.HTTP.Protocol(), HTTPPort),
		"ent_search.listen_host":         "0.0.0.0",
		"filebeat_log_directory":         LogVolumeMountPath,
		"log_directory":                  LogVolumeMountPath,
		"allow_es_settings_modification": true,
	})
}

func associationConfig(c k8s.Client, ent entv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	if !ent.AssociationConf().IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	username, password, err := association.ElasticsearchAuthSettings(c, &ent)
	if err != nil {
		return nil, err
	}
	cfg := settings.MustCanonicalConfig(map[string]string{
		"ent_search.auth.source": "elasticsearch-native",
		"elasticsearch.host":     ent.AssociationConf().URL,
		"elasticsearch.username": username,
		"elasticsearch.password": password,
	})

	if ent.AssociationConf().CAIsConfigured() {
		if err := cfg.MergeWith(settings.MustCanonicalConfig(map[string]interface{}{
			"elasticsearch.ssl.enabled":               true,
			"elasticsearch.ssl.certificate_authority": filepath.Join(ESCertsPath, certificates.CertFileName),
		})); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func tlsConfig(ent entv1beta1.EnterpriseSearch) *settings.CanonicalConfig {
	if !ent.Spec.HTTP.TLS.Enabled() {
		return settings.NewCanonicalConfig()
	}
	certsDir := certificates.HTTPCertSecretVolume(name.EntNamer, ent.Name).VolumeMount().MountPath
	return settings.MustCanonicalConfig(map[string]interface{}{
		"ent_search.ssl.enabled":                 true,
		"ent_search.ssl.certificate":             filepath.Join(certsDir, certificates.CertFileName),
		"ent_search.ssl.key":                     filepath.Join(certsDir, certificates.KeyFileName),
		"ent_search.ssl.certificate_authorities": []string{filepath.Join(certsDir, certificates.CAFileName)},
	})
}
