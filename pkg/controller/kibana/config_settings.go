// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"context"
	"path"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/elastic/go-ucfg"
	"github.com/pkg/errors"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// SettingsFilename is the Kibana configuration settings file
	SettingsFilename = "kibana.yml"
	// EnvNodeOpts is the environment variable name for the Node options that can be used to increase the Kibana maximum memory limit
	EnvNodeOpts = "NODE_OPTS"

	// esCertsVolumeMountPath is the directory containing Elasticsearch certificates.
	esCertsVolumeMountPath = "/usr/share/kibana/config/elasticsearch-certs"
)

// Constants to use for the Kibana configuration settings.
const (
	ServerName                                     = "server.name"
	ServerHost                                     = "server.host"
	XpackMonitoringUIContainerElasticsearchEnabled = "xpack.monitoring.ui.container.elasticsearch.enabled"
	XpackLicenseManagementUIEnabled                = "xpack.license_management.ui.enabled" // >= 7.6
	XpackSecurityEncryptionKey                     = "xpack.security.encryptionKey"
	XpackReportingEncryptionKey                    = "xpack.reporting.encryptionKey"
	XpackEncryptedSavedObjects                     = "xpack.encryptedSavedObjects"
	XpackEncryptedSavedObjectsEncryptionKey        = "xpack.encryptedSavedObjects.encryptionKey"

	ElasticsearchSslCertificateAuthorities = "elasticsearch.ssl.certificateAuthorities"
	ElasticsearchSslVerificationMode       = "elasticsearch.ssl.verificationMode"

	ElasticsearchUsername = "elasticsearch.username"
	ElasticsearchPassword = "elasticsearch.password"

	ElasticsearchHosts = "elasticsearch.hosts"

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

	specConfig := kb.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}

	userSettings, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return CanonicalConfig{}, err
	}

	cfg := settings.MustCanonicalConfig(baseSettings(&kb, ipFamily))
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
		filteredReusableSettings,
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

// Some previously-unsupported keys cause Kibana to error out even if the values are empty. ucfg cannot ignore fields easily so this is necessary to
// support older versions
func filterConfigSettings(kb kbv1.Kibana, cfg *settings.CanonicalConfig) (*settings.CanonicalConfig, error) {
	ver, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return cfg, err
	}
	if !ver.IsSameOrAfter(version.From(7, 6, 0)) {
		_, err = (*ucfg.Config)(cfg).Remove(XpackEncryptedSavedObjects, -1, settings.Options...)
	}
	return cfg, err
}

// VersionDefaults generates any version specific settings that should exist by default.
func VersionDefaults(kb *kbv1.Kibana, v version.Version) *settings.CanonicalConfig {
	if v.IsSameOrAfter(version.From(7, 6, 0)) {
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
	if len(r.SavedObjectsKey) == 0 && kbVer.IsSameOrAfter(version.From(7, 6, 0)) {
		r.SavedObjectsKey = string(common.RandomBytes(64))
	}
	return settings.MustCanonicalConfig(r), nil
}

func baseSettings(kb *kbv1.Kibana, ipFamily corev1.IPFamily) map[string]interface{} {
	conf := map[string]interface{}{
		ServerName: kb.Name,
		ServerHost: net.InAddrAnyFor(ipFamily).String(),
		XpackMonitoringUIContainerElasticsearchEnabled: true,
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
		esCertsVolumeMountPath := esCaCertSecretVolume(kb).VolumeMount().MountPath
		cfg[ElasticsearchSslCertificateAuthorities] = path.Join(esCertsVolumeMountPath, certificates.CAFileName)
	}

	return cfg
}

// esCaCertSecretVolume returns a SecretVolume to hold the Elasticsearch CA certs for the given Kibana resource.
func esCaCertSecretVolume(kb kbv1.Kibana) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		kb.AssociationConf().GetCASecretName(),
		"elasticsearch-certs",
		esCertsVolumeMountPath,
	)
}
