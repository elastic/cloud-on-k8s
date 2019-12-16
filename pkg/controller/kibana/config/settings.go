// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"path"

	"github.com/pkg/errors"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/es"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

// Kibana configuration settings file
const SettingsFilename = "kibana.yml"

// CanonicalConfig contains configuration for Kibana ("kibana.yml"),
// as a hierarchical key-value configuration.
type CanonicalConfig struct {
	*settings.CanonicalConfig
}

// NewConfigSettings returns the Kibana configuration settings for the given Kibana resource.
// TODO sabo does it belong here?
func NewConfigSettings(client k8s.Client, kb kbv1.Kibana, versionSpecificCfg *settings.CanonicalConfig) (CanonicalConfig, error) {

	// currentConfig, _ := getExistingConfig(client, kb)
	specConfig := kb.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}

	userSettings, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	cfg := settings.MustCanonicalConfig(baseSettings(kb))
	kibanaTLSCfg := settings.MustCanonicalConfig(kibanaTLSSettings(kb))

	if !kb.RequiresAssociation() {
		if err := cfg.MergeWith(versionSpecificCfg, kibanaTLSCfg, userSettings); err != nil {
			return CanonicalConfig{}, err
		}
		return CanonicalConfig{cfg}, nil
	}

	username, password, err := association.ElasticsearchAuthSettings(client, &kb)
	if err != nil {
		return CanonicalConfig{}, err
	}

	// merge the configuration with userSettings last so they take precedence
	err = cfg.MergeWith(
		versionSpecificCfg,
		kibanaTLSCfg,
		settings.MustCanonicalConfig(elasticsearchTLSSettings(kb)),
		settings.MustCanonicalConfig(
			map[string]interface{}{
				ElasticsearchUsername: username,
				ElasticsearchPassword: password,
			},
		),
		userSettings,
	)
	if err != nil {
		return CanonicalConfig{}, err
	}

	return CanonicalConfig{cfg}, nil
}

// getExistingConfig retrieves the canonical config for a given Kibana, if one exists
func getExistingConfig(client k8s.Client, kb kbv1.Kibana) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	err := client.Get(types.NamespacedName{Name: SecretName(kb), Namespace: kb.Namespace}, &secret)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rawCfg, exists := secret.Data[SettingsFilename]
	if !exists {
		return nil, errors.Errorf("No key %s in secret %s/%s", SettingsFilename, secret.Namespace, secret.Name)
	}
	return settings.ParseConfig(rawCfg)
}

func baseSettings(kb kbv1.Kibana) map[string]interface{} {
	return map[string]interface{}{
		ServerName:         kb.Name,
		ServerHost:         "0",
		ElasticSearchHosts: []string{kb.AssociationConf().GetURL()},
		XpackMonitoringUiContainerElasticsearchEnabled: true,
	}
}

func kibanaTLSSettings(kb kbv1.Kibana) map[string]interface{} {
	if !kb.Spec.HTTP.TLS.Enabled() {
		return nil
	}
	return map[string]interface{}{
		ServerSSLEnabled:     true,
		ServerSSLCertificate: path.Join(http.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),
		ServerSSLKey:         path.Join(http.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
	}
}

func elasticsearchTLSSettings(kb kbv1.Kibana) map[string]interface{} {
	cfg := map[string]interface{}{
		ElasticsearchSslVerificationMode: "certificate",
	}

	if kb.AssociationConf().GetCACertProvided() {
		esCertsVolumeMountPath := es.CaCertSecretVolume(kb).VolumeMount().MountPath
		cfg[ElasticsearchSslCertificateAuthorities] = path.Join(esCertsVolumeMountPath, certificates.CAFileName)
	}

	return cfg
}

// ensureEncryptionKey ensures a given config contains an encryption key
func ensureEncryptionKey(cfg map[string]interface{}) {
	if _, ok := cfg[XpackSecurityEncryptionKey]; !ok {
		cfg[XpackSecurityEncryptionKey] = rand.String(64)
	}
}
