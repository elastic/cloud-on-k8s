// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"context"
	"path"

	"github.com/pkg/errors"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/es"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// SettingsFilename is the Kibana configuration settings file
	SettingsFilename = "kibana.yml"
	// EnvNodeOpts is the environment variable name for the Node options that can be used to increase the Kibana maximum memory limit
	EnvNodeOpts = "NODE_OPTS"
)

var log = logf.Log.WithName("kibana-config")

// CanonicalConfig contains configuration for Kibana ("kibana.yml"),
// as a hierarchical key-value configuration.
type CanonicalConfig struct {
	*settings.CanonicalConfig
}

// NewConfigSettings returns the Kibana configuration settings for the given Kibana resource.
func NewConfigSettings(ctx context.Context, client k8s.Client, kb kbv1.Kibana, v version.Version) (CanonicalConfig, error) {
	span, _ := apm.StartSpan(ctx, "new_config_settings", tracing.SpanTypeApp)
	defer span.End()

	reusableSettings, err := getOrCreateReusableSettings(client, kb)
	if err != nil {
		return CanonicalConfig{}, err
	}

	specConfig := kb.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}

	userSettings, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	cfg := settings.MustCanonicalConfig(baseSettings(&kb))
	kibanaTLSCfg := settings.MustCanonicalConfig(kibanaTLSSettings(kb))
	versionSpecificCfg := VersionDefaults(&kb, v)

	if !kb.RequiresAssociation() {
		// merge the configuration with userSettings last so they take precedence
		if err := cfg.MergeWith(
			reusableSettings,
			versionSpecificCfg,
			kibanaTLSCfg,
			userSettings); err != nil {
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
		reusableSettings,
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

// reusableSettings captures secrets settings in the Kibana configuration that we want to reuse.
type reusableSettings struct {
	EncryptionKey string `config:"xpack.security.encryptionKey"`
}

// getExistingConfig retrieves the canonical config for a given Kibana, if one exists
func getExistingConfig(client k8s.Client, kb kbv1.Kibana) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	err := client.Get(types.NamespacedName{Name: SecretName(kb), Namespace: kb.Namespace}, &secret)
	if err != nil && apierrors.IsNotFound(err) {
		log.V(1).Info("Kibana config secret does not exist", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return nil, nil
	} else if err != nil {
		log.Error(err, "Error retrieving kibana config secret", "namespace", kb.Namespace, "kibana_name", kb.Name)
		return nil, err
	}
	rawCfg, exists := secret.Data[SettingsFilename]
	if !exists {
		err = errors.New("Kibana config secret exists but missing config file key")
		log.Error(err, "", "namespace", secret.Namespace, "secret_name", secret.Name, "key", SettingsFilename)
		return nil, err
	}
	cfg, err := settings.ParseConfig(rawCfg)
	if err != nil {
		log.Error(err, "Error parsing existing kibana config in secret", "namespace", secret.Namespace, "secret_name", secret.Name, "key", SettingsFilename)
		return nil, err
	}
	return cfg, nil
}

// getOrCreateReusableSettings filters an existing config for only items we want to preserve between spec changes
// because they cannot be generated deterministically, e.g. encryption keys
func getOrCreateReusableSettings(c k8s.Client, kb kbv1.Kibana) (*settings.CanonicalConfig, error) {
	cfg, err := getExistingConfig(c, kb)
	if err != nil {
		return nil, err
	}

	var r reusableSettings
	if cfg == nil {
		r = reusableSettings{}
	} else if err := cfg.Unpack(&r); err != nil {
		return nil, err
	}
	if len(r.EncryptionKey) == 0 {
		r.EncryptionKey = string(common.RandomBytes(64))
	}
	return settings.MustCanonicalConfig(r), nil
}

func baseSettings(kb *kbv1.Kibana) map[string]interface{} {
	conf := map[string]interface{}{
		ServerName: kb.Name,
		ServerHost: "0",
		XpackMonitoringUiContainerElasticsearchEnabled: true,
	}

	if kb.RequiresAssociation() {
		conf[ElasticsearchHosts] = []string{kb.AssociationConf().GetURL()}
	}

	return conf
}

func kibanaTLSSettings(kb kbv1.Kibana) map[string]interface{} {
	if !kb.Spec.HTTP.TLS.Enabled() {
		return nil
	}
	return map[string]interface{}{
		ServerSSLEnabled:     true,
		ServerSSLCertificate: path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),
		ServerSSLKey:         path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
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
