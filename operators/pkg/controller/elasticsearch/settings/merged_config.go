// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	commonsettings "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived  from the given
// parameters.
func NewMergedESConfig(
	clusterName string,
	userConfig v1alpha1.Config,
) (*commonsettings.CanonicalConfig, error) {
	config, err := commonsettings.NewCanonicalConfigFrom(userConfig.Data)
	if err != nil {
		return nil, err
	}
	err = config.MergeWith(
		baseConfig(clusterName),
		xpackConfig(),
	)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// baseConfig returns the base ES configuration to apply for the given cluster
func baseConfig(clusterName string) *commonsettings.CanonicalConfig {
	return commonsettings.MustCanonicalConfig(map[string]interface{}{
		// derive node name dynamically from the pod name, injected as env var
		NodeName:    "${" + EnvPodName + "}",
		ClusterName: clusterName,

		DiscoveryZenHostsProvider: "file",

		// derive IP dynamically from the pod IP, injected as env var
		NetworkPublishHost: "${" + EnvPodIP + "}",
		NetworkHost:        "0.0.0.0",

		PathData: initcontainer.DataSharedVolume.EsContainerMountPath,
		PathLogs: initcontainer.LogsSharedVolume.EsContainerMountPath,
	})
}

// xpackConfig returns the configuration bit related to XPack settings
func xpackConfig() *commonsettings.CanonicalConfig {
	// enable x-pack security, including TLS
	cfg := map[string]interface{}{
		// x-pack security general settings
		XPackSecurityEnabled:                      "true",
		XPackSecurityAuthcReservedRealmEnabled:    "false",
		XPackSecurityTransportSslVerificationMode: "certificate",

		// x-pack security http settings
		XPackSecurityHttpSslEnabled:     "true",
		XPackSecurityHttpSslKey:         path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
		XPackSecurityHttpSslCertificate: path.Join(volume.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),

		// x-pack security transport settings
		XPackSecurityTransportSslEnabled:                "true",
		XPackSecurityTransportSslKey:                    path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		XPackSecurityTransportSslCertificate:            path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CertFileName),
		XPackSecurityTransportSslCertificateAuthorities: path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CAFileName),

		// x-pack security transport ssl trust restrictions settings
		XPackSecurityTransportSslTrustRestrictionsPath: path.Join(
			volume.TransportCertificatesSecretVolumeMountPath,
			transport.TrustRestrictionsFilename,
		),
	}

	return commonsettings.MustCanonicalConfig(cfg)
}
