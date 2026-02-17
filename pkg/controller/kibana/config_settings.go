// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	commonassociation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
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
	// eprCertsVolumeMountPath is the directory into which trusted Package Registry CA certs are mounted.
	eprCertsVolumeMountPath = "/usr/share/kibana/config/epr-certs"

	// EncryptionKeyMinimumBytes is the minimum number of bytes required for the encryption key.
	// This is in line with the documentation (32 characters) as of 9.0 (unicode characters can use > 1 byte):
	// https://www.elastic.co/guide/en/kibana/9.0/using-kibana-with-security.html#security-configure-settings
	EncryptionKeyMinimumBytes = 64
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
	XpackFleetRegistryURL                          = "xpack.fleet.registryUrl"
	XpackFleetPackages                             = "xpack.fleet.packages"
	XpackFleetOutputs                              = "xpack.fleet.outputs"
	XpackFleetAgents                               = "xpack.fleet.agents"
	XpackFleetAgentsElasticsearch                  = "xpack.fleet.agents.elasticsearch"
	XpackFleetAgentsElasticsearchHosts             = "xpack.fleet.agents.elasticsearch.hosts"
	ECKFleetOutputID                               = "eck-fleet-agent-output-elasticsearch"
	ECKFleetOutputName                             = "eck-elasticsearch"

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

// fleetOutputSSL represents the ssl configuration for an xpack Fleet output.
type fleetOutputSSL struct {
	CertificateAuthorities []string `config:"certificate_authorities"`
}

// fleetOutput represents an xpack Fleet output configuration.
type fleetOutput struct {
	ID        string          `config:"id"`
	IsDefault bool            `config:"is_default"`
	Name      string          `config:"name"`
	Type      string          `config:"type"`
	Hosts     []string        `config:"hosts"`
	SSL       *fleetOutputSSL `config:"ssl,omitempty"`
}

// fleetOutputsConfig represents the xpack.fleet.outputs configuration.
type fleetOutputsConfig struct {
	Outputs []fleetOutput `config:"xpack.fleet.outputs"`
}

// fleetAgentsConfig represents the xpack.fleet.agents configuration.
type fleetAgentsConfig struct {
	Agents map[string]any `config:"xpack.fleet.agents"`
}

// NewConfigSettings returns the Kibana configuration settings for the given Kibana resource.
func NewConfigSettings(ctx context.Context, client k8s.Client, kb kbv1.Kibana, v version.Version, ipFamily corev1.IPFamily, kibanaConfigFromPolicy *settings.CanonicalConfig) (CanonicalConfig, error) {
	span, _ := apm.StartSpan(ctx, "new_config_settings", tracing.SpanTypeApp)
	defer span.End()

	reusableSettings, err := getOrCreateReusableSettings(ctx, client, kb)
	if err != nil {
		return CanonicalConfig{}, err
	}

	// hack to support pre-7.6.0 Kibana configs as it errors out with unsupported keys, ideally we would not unpack empty values and could skip this
	err = filterConfigSettings(kb, reusableSettings)
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
	eprCfg := settings.MustCanonicalConfig(packageRegistrySettings(kb))
	monitoringCfg, err := settings.NewCanonicalConfigFrom(stackmon.MonitoringConfig(kb).Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	err = cfg.MergeWith(
		reusableSettings,
		versionSpecificCfg,
		kibanaTLSCfg,
		entSearchCfg,
		monitoringCfg,
		eprCfg)
	if err != nil {
		return CanonicalConfig{}, err
	}

	// Elasticsearch configuration
	esAssocConf, err := kb.EsAssociation().AssociationConf()
	if err != nil {
		return CanonicalConfig{}, err
	}
	if esAssocConf.IsConfigured() {
		credentials, err := association.ElasticsearchAuthSettings(ctx, client, kb.EsAssociation())
		if err != nil {
			return CanonicalConfig{}, err
		}
		var esCreds map[string]any
		if credentials.HasServiceAccountToken() {
			esCreds = map[string]any{
				ElasticsearchServiceAccountToken: credentials.ServiceAccountToken,
			}
		} else {
			esCreds = map[string]any{
				ElasticsearchUsername: credentials.Username,
				ElasticsearchPassword: credentials.Password,
			}
		}
		credentialsCfg := settings.MustCanonicalConfig(esCreds)
		esAssocCfg := settings.MustCanonicalConfig(elasticsearchTLSSettings(*esAssocConf))
		if err = cfg.MergeWith(esAssocCfg, credentialsCfg); err != nil {
			return CanonicalConfig{}, err
		}
	}

	// Kibana settings from a StackConfigPolicy takes precedence over user provided settings, merge them last.
	if err = cfg.MergeWith(userSettings, kibanaConfigFromPolicy); err != nil {
		return CanonicalConfig{}, err
	}
	if err = maybeConfigureFleetOutputs(cfg, esAssocConf, kb.EsAssociation()); err != nil {
		return CanonicalConfig{}, err
	}

	return CanonicalConfig{cfg}, nil
}

// maybeConfigureFleetOutputs potentially adds a default xpack.fleet.outputs block when no outputs are configured and ensure xpack.fleet.agents.elasticsearch is removed when no agents are configured.
func maybeConfigureFleetOutputs(cfg *settings.CanonicalConfig, esAssocConf *commonv1.AssociationConf, esAssoc commonv1.Association) error {
	var fleetCfg fleetOutputsConfig
	if err := cfg.Unpack(&fleetCfg); err != nil {
		return err
	}

	if len(fleetCfg.Outputs) > 0 {
		return removeXPackFleetAgentsElasticsearch(cfg)
	}

	if len(fleetCfg.Outputs) == 0 {
		if esAssocConf != nil && esAssocConf.IsConfigured() && hasFleetConfigured(cfg) {
			if err := cfg.MergeWith(defaultFleetOutputsConfig(*esAssocConf, esAssoc)); err != nil {
				return err
			}
			return removeXPackFleetAgentsElasticsearch(cfg)
		}
	}

	return nil
}

// removeXPackFleetAgentsElasticsearch removes xpack.fleet.agents.elasticsearch and
// prunes xpack.fleet.agents when it becomes empty avoiding null entries. (xpack.fleet.agents: null)
func removeXPackFleetAgentsElasticsearch(cfg *settings.CanonicalConfig) error {
	if err := cfg.Remove(XpackFleetAgentsElasticsearch); err != nil {
		return err
	}
	var fleetCfg fleetAgentsConfig
	if err := cfg.Unpack(&fleetCfg); err != nil {
		return err
	}
	if len(fleetCfg.Agents) == 0 {
		return cfg.Remove(XpackFleetAgents)
	}
	return nil
}

// hasFleetConfigured returns true when any xpack.fleet.* setting are present in the effective config.
func hasFleetConfigured(cfg *settings.CanonicalConfig) bool {
	return cfg.HasChildConfig("xpack.fleet")
}

// defaultFleetOutputsConfig builds the default xpack.fleet.outputs block from an ES association.
func defaultFleetOutputsConfig(esAssocConf commonv1.AssociationConf, esAssoc commonv1.Association) *settings.CanonicalConfig {
	output := fleetOutput{
		ID:        ECKFleetOutputID,
		IsDefault: true,
		Name:      ECKFleetOutputName,
		Type:      "elasticsearch",
		Hosts:     []string{esAssocConf.GetURL()},
	}
	if esAssocConf.GetCACertProvided() {
		esCertsPath := path.Join(commonassociation.CertificatesDir(esAssoc), certificates.CAFileName)
		output.SSL = &fleetOutputSSL{
			CertificateAuthorities: []string{esCertsPath},
		}
	}
	var fleetCfg fleetOutputsConfig
	fleetCfg.Outputs = []fleetOutput{output}

	return settings.MustCanonicalConfig(fleetCfg)
}

// Some previously-unsupported keys cause Kibana to error out even if the values are empty. ucfg cannot ignore fields easily so this is necessary to
// support older versions
func filterConfigSettings(kb kbv1.Kibana, cfg *settings.CanonicalConfig) error {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return err
	}
	if !ver.GTE(version.From(7, 6, 0)) {
		err = cfg.Remove(XpackEncryptedSavedObjects)
	}
	return err
}

// VersionDefaults generates any version specific settings that should exist by default.
func VersionDefaults(_ *kbv1.Kibana, v version.Version) *settings.CanonicalConfig {
	if v.GTE(version.From(7, 6, 0)) {
		// setting exists only as of 7.6.0
		return settings.MustCanonicalConfig(map[string]any{XpackLicenseManagementUIEnabled: false})
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
func getExistingConfig(ctx context.Context, client k8s.Client, kb kbv1.Kibana) (*settings.CanonicalConfig, error) {
	log := ulog.FromContext(ctx)
	var secret corev1.Secret
	err := client.Get(context.Background(), types.NamespacedName{Name: kbv1.ConfigSecret(kb.Name), Namespace: kb.Namespace}, &secret)
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
func getOrCreateReusableSettings(ctx context.Context, c k8s.Client, kb kbv1.Kibana) (*settings.CanonicalConfig, error) {
	cfg, err := getExistingConfig(ctx, c, kb)
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
		// This is generated without symbols to stay in line with Elasticsearch's service accounts
		// which are UUIDv4 and cannot include symbols.
		bytes, err := commonpassword.RandomBytesWithoutSymbols(EncryptionKeyMinimumBytes)
		if err != nil {
			return nil, err
		}
		r.EncryptionKey = string(bytes)
	}
	if len(r.ReportingKey) == 0 {
		// This is generated without symbols to stay in line with Elasticsearch's service accounts
		// which are UUIDv4 and cannot include symbols.
		bytes, err := commonpassword.RandomBytesWithoutSymbols(EncryptionKeyMinimumBytes)
		if err != nil {
			return nil, err
		}
		r.ReportingKey = string(bytes)
	}

	kbVer, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return nil, err
	}
	// xpack.encryptedSavedObjects.encryptionKey was only added in 7.6.0 and earlier versions error out
	if len(r.SavedObjectsKey) == 0 && kbVer.GTE(version.From(7, 6, 0)) {
		// This is generated without symbols to stay in line with Elasticsearch's service accounts
		// which are UUIDv4 and cannot include symbols.
		bytes, err := commonpassword.RandomBytesWithoutSymbols(EncryptionKeyMinimumBytes)
		if err != nil {
			return nil, err
		}
		r.SavedObjectsKey = string(bytes)
	}
	return settings.MustCanonicalConfig(r), nil
}

func baseSettings(kb *kbv1.Kibana, ipFamily corev1.IPFamily) (map[string]any, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return nil, err
	}

	conf := map[string]any{
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

func kibanaTLSSettings(kb kbv1.Kibana) map[string]any {
	if !kb.Spec.HTTP.TLS.Enabled() {
		return nil
	}
	return map[string]any{
		ServerSSLEnabled:     true,
		ServerSSLCertificate: path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),
		ServerSSLKey:         path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
	}
}

func elasticsearchTLSSettings(esAssocConf commonv1.AssociationConf) map[string]any {
	cfg := map[string]any{
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

// eprCaCertSecretVolume returns a SecretVolume to hold the Elastic Package Registry CA certs for the given Kibana resource.
func eprCaCertSecretVolume(eprAssocConf commonv1.AssociationConf) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		eprAssocConf.GetCASecretName(),
		"epr-certs",
		eprCertsVolumeMountPath,
	)
}

func enterpriseSearchSettings(kb kbv1.Kibana) map[string]any {
	cfg := map[string]any{}
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

func packageRegistrySettings(kb kbv1.Kibana) map[string]any {
	cfg := map[string]any{}
	assocConf, _ := kb.EPRAssociation().AssociationConf()
	if assocConf.URLIsConfigured() {
		cfg[XpackFleetRegistryURL] = assocConf.GetURL()
	}
	return cfg
}
