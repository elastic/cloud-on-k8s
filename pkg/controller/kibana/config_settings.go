// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"path"
	"path/filepath"

	"github.com/elastic/go-ucfg"
	"github.com/pkg/errors"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/stackmon"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

const (
	// SettingsFilename is the Kibana configuration settings file
	SettingsFilename = "kibana.yml"
	// EnvNodeOptions is the environment variable name for the Node options that can be used to increase the Kibana maximum memory limit
	EnvNodeOptions = "NODE_OPTIONS"

	// esCertsVolumeMountPath is the directory containing Elasticsearch certificates.
	esCertsVolumeMountPath = "/usr/share/kibana/config/elasticsearch-certs"
	// entCertsVolumeMountPath is the directory into which trusted Enterprise Search HTTP CA certs are mounted.
	entCertsVolumeMountPath = "/usr/share/kibana/config/ent-certs"
)

// Constants to use for the Kibana configuration settings.
const (
	ServerName                                     = "server.name"
	ServerHost                                     = "server.host"
	XpackMonitoringUIContainerElasticsearchEnabled = "xpack.monitoring.ui.container.elasticsearch.enabled" // <= 7.15
	MonitoringUIContainerElasticsearchEnabled      = "monitoring.ui.container.elasticsearch.enabled"       // >= 7.16
	XpackLicenseManagementUIEnabled                = "xpack.license_management.ui.enabled"                 // >= 7.6
	XpackSecurityEncryptionKey                     = "xpack.security.encryptionKey"
	XpackReportingEncryptionKey                    = "xpack.reporting.encryptionKey"
	XpackEncryptedSavedObjects                     = "xpack.encryptedSavedObjects"
	XpackEncryptedSavedObjectsEncryptionKey        = "xpack.encryptedSavedObjects.encryptionKey"

	ElasticsearchSslCertificateAuthorities = "elasticsearch.ssl.certificateAuthorities"
	ElasticsearchSslVerificationMode       = "elasticsearch.ssl.verificationMode"

	ElasticsearchUsername            = "elasticsearch.username"
	ElasticsearchPassword            = "elasticsearch.password"
	ElasticsearchServiceAccountToken = "elasticsearch.serviceAccountToken"

	ElasticsearchHosts = "elasticsearch.hosts"

	EnterpriseSearchHost                      = "enterpriseSearch.host"
	EnterpriseSearchSslCertificateAuthorities = "enterpriseSearch.ssl.certificateAuthorities"
	EnterpriseSearchSslVerificationMode       = "enterpriseSearch.ssl.verificationMode"

	ServerSSLEnabled     = "server.ssl.enabled"
	ServerSSLCertificate = "server.ssl.certificate"
	ServerSSLKey         = "server.ssl.key"
)

// CanonicalConfig contains configuration for Kibana ("kibana.yml"),
// as a hierarchical key-value configuration.
type CanonicalConfig struct {
	*settings.CanonicalConfig
}

// NewConfigSettings returns the Kibana configuration settings for the given Kibana resource.
func NewConfigSettings(ctx context.Context, client k8s.Client, kb kbv1.Kibana, v version.Version, ipFamily corev1.IPFamily) (CanonicalConfig, error) {
	span, _ := apm.StartSpan(ctx, "new_config_settings", tracing.SpanTypeApp)
	defer span.End()

	reusableSettings, err := getOrCreateReusableSettings(client, kb)
	if err != nil {
		return CanonicalConfig{}, err
	}

	// hack to support pre-7.6.0 Kibana configs as it errors out with unsupported keys, ideally we would not unpack empty values and could skip this
	filteredReusableSettings, err := filterConfigSettings(kb, reusableSettings)
	if err != nil {
		return CanonicalConfig{}, err
	}

	// parse user-provided settings
	specConfig := kb.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}
	userSettings, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	baseSettingsMap, err := baseSettings(&kb, ipFamily)
	if err != nil {
		return CanonicalConfig{}, err
	}

	cfg := settings.MustCanonicalConfig(baseSettingsMap)
	kibanaTLSCfg := settings.MustCanonicalConfig(kibanaTLSSettings(kb))
	versionSpecificCfg := VersionDefaults(&kb, v)
	entSearchCfg := settings.MustCanonicalConfig(enterpriseSearchSettings(kb))
	monitoringCfg, err := settings.NewCanonicalConfigFrom(stackmon.MonitoringConfig(kb).Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	esAssocConf, err := kb.EsAssociation().AssociationConf()
	if err != nil {
		return CanonicalConfig{}, err
	}
	if !esAssocConf.IsConfigured() {
		// merge the configuration with userSettings last so they take precedence
		if err := cfg.MergeWith(
			reusableSettings,
			versionSpecificCfg,
			kibanaTLSCfg,
			entSearchCfg,
			monitoringCfg,
			userSettings); err != nil {
			return CanonicalConfig{}, err
		}
		return CanonicalConfig{cfg}, nil
	}

	credentials, err := association.ElasticsearchAuthSettings(client, kb.EsAssociation())
	if err != nil {
		return CanonicalConfig{}, err
	}
	var credentialsCfg *settings.CanonicalConfig
	if credentials.HasServiceAccountToken() {
		credentialsCfg =
			settings.MustCanonicalConfig(
				map[string]interface{}{
					ElasticsearchServiceAccountToken: credentials.ServiceAccountToken,
				},
			)
	} else {
		credentialsCfg =
			settings.MustCanonicalConfig(
				map[string]interface{}{
					ElasticsearchUsername: credentials.Username,
					ElasticsearchPassword: credentials.Password,
				},
			)
	}

	// merge the configuration with userSettings last so they take precedence
	err = cfg.MergeWith(
		filteredReusableSettings,
		versionSpecificCfg,
		kibanaTLSCfg,
		entSearchCfg,
		monitoringCfg,
		settings.MustCanonicalConfig(elasticsearchTLSSettings(*esAssocConf)),
		credentialsCfg,
		userSettings,
	)
	if err != nil {
		return CanonicalConfig{}, err
	}

	return CanonicalConfig{cfg}, nil
}

// Some previously-unsupported keys cause Kibana to error out even if the values are empty. ucfg cannot ignore fields easily so this is necessary to
// support older versions
func filterConfigSettings(kb kbv1.Kibana, cfg *settings.CanonicalConfig) (*settings.CanonicalConfig, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return cfg, err
	}
	if !ver.GTE(version.From(7, 6, 0)) {
		_, err = (*ucfg.Config)(cfg).Remove(XpackEncryptedSavedObjects, -1, settings.Options...)
	}
	return cfg, err
}

// VersionDefaults generates any version specific settings that should exist by default.
func VersionDefaults(kb *kbv1.Kibana, v version.Version) *settings.CanonicalConfig {
	if v.GTE(version.From(7, 6, 0)) {
		// setting exists only as of 7.6.0
		return settings.MustCanonicalConfig(map[string]interface{}{XpackLicenseManagementUIEnabled: false})
	}

	return settings.NewCanonicalConfig()
}

// reusableSettings captures secrets settings in the Kibana configuration that we want to reuse.
type reusableSettings struct {
	EncryptionKey   string `config:"xpack.security.encryptionKey"`
	ReportingKey    string `config:"xpack.reporting.encryptionKey"`
	SavedObjectsKey string `config:"xpack.encryptedSavedObjects.encryptionKey"`
}

// getExistingConfig retrieves the canonical config for a given Kibana, if one exists
func getExistingConfig(client k8s.Client, kb kbv1.Kibana) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	err := client.Get(context.Background(), types.NamespacedName{Name: SecretName(kb), Namespace: kb.Namespace}, &secret)
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
	if len(r.ReportingKey) == 0 {
		r.ReportingKey = string(common.RandomBytes(64))
	}

	kbVer, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return nil, err
	}
	// xpack.encryptedSavedObjects.encryptionKey was only added in 7.6.0 and earlier versions error out
	if len(r.SavedObjectsKey) == 0 && kbVer.GTE(version.From(7, 6, 0)) {
		r.SavedObjectsKey = string(common.RandomBytes(64))
	}
	return settings.MustCanonicalConfig(r), nil
}

func baseSettings(kb *kbv1.Kibana, ipFamily corev1.IPFamily) (map[string]interface{}, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return nil, err
	}

	conf := map[string]interface{}{
		ServerName: kb.Name,
		ServerHost: net.InAddrAnyFor(ipFamily).String(),
	}

	if ver.GTE(version.MinFor(7, 16, 0)) {
		conf[MonitoringUIContainerElasticsearchEnabled] = true
	} else {
		conf[XpackMonitoringUIContainerElasticsearchEnabled] = true
	}

	assocConf, _ := kb.EsAssociation().AssociationConf()
	if assocConf.URLIsConfigured() {
		conf[ElasticsearchHosts] = []string{assocConf.GetURL()}
	}

	return conf, nil
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

func elasticsearchTLSSettings(esAssocConf commonv1.AssociationConf) map[string]interface{} {
	cfg := map[string]interface{}{
		ElasticsearchSslVerificationMode: "certificate",
	}

	if esAssocConf.GetCACertProvided() {
		esCertsVolumeMountPath := esCaCertSecretVolume(esAssocConf).VolumeMount().MountPath
		cfg[ElasticsearchSslCertificateAuthorities] = path.Join(esCertsVolumeMountPath, certificates.CAFileName)
	}

	return cfg
}

// esCaCertSecretVolume returns a SecretVolume to hold the Elasticsearch CA certs for the given Kibana resource.
func esCaCertSecretVolume(esAssocConf commonv1.AssociationConf) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		esAssocConf.GetCASecretName(),
		"elasticsearch-certs",
		esCertsVolumeMountPath,
	)
}

// entCaCertSecretVolume returns a SecretVolume to hold the EnterpriseSearch CA certs for the given Kibana resource.
func entCaCertSecretVolume(entAssocConf commonv1.AssociationConf) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		entAssocConf.GetCASecretName(),
		"ent-certs",
		entCertsVolumeMountPath,
	)
}

func enterpriseSearchSettings(kb kbv1.Kibana) map[string]interface{} {
	cfg := map[string]interface{}{}
	assocConf, _ := kb.EntAssociation().AssociationConf()
	if assocConf.URLIsConfigured() {
		cfg[EnterpriseSearchHost] = assocConf.GetURL()
	}
	if assocConf.GetCACertProvided() {
		cfg[EnterpriseSearchSslCertificateAuthorities] = filepath.Join(entCertsVolumeMountPath, certificates.CAFileName)
		// Rely on "certificate" verification mode rather than "full" to allow Kibana
		// to connect to Enterprise Search through the k8s-internal service DNS name
		// even though the user-provided certificate may only specify a public-facing DNS.
		cfg[EnterpriseSearchSslVerificationMode] = "certificate"
	}
	return cfg
}
