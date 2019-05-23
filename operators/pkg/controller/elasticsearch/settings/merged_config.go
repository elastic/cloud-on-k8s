// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

// NewMergedESConfig merges user provided Elasticsearch configuration with configuration derived  from the given
// parameters.
func NewMergedESConfig(
	clusterName string,
	userConfig v1alpha1.Config,
) (*CanonicalConfig, error) {
	config, err := NewCanonicalConfigFrom(userConfig)
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
func baseConfig(clusterName string) *CanonicalConfig {
	return MustCanonicalConfig(map[string]interface{}{
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
func xpackConfig() *CanonicalConfig {
	// enable x-pack security, including TLS
	cfg := map[string]interface{}{
		// x-pack security general settings
		XPackSecurityEnabled:                      "true",
		XPackSecurityAuthcReservedRealmEnabled:    "false",
		XPackSecurityTransportSslVerificationMode: "certificate",

		// x-pack security http settings
		XPackSecurityHttpSslEnabled:                "true",
		XPackSecurityHttpSslKey:                    path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		XPackSecurityHttpSslCertificate:            path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CertFileName),
		XPackSecurityHttpSslCertificateAuthorities: path.Join(volume.NodeCertificatesSecretVolumeMountPath, certificates.CAFileName),

		// x-pack security transport settings
		XPackSecurityTransportSslEnabled:                "true",
		XPackSecurityTransportSslKey:                    path.Join(initcontainer.PrivateKeySharedVolume.EsContainerMountPath, initcontainer.PrivateKeyFileName),
		XPackSecurityTransportSslCertificate:            path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CertFileName),
		XPackSecurityTransportSslCertificateAuthorities: path.Join(volume.NodeCertificatesSecretVolumeMountPath, certificates.CAFileName),

		// x-pack security transport ssl trust restrictions settings
		XPackSecurityTransportSslTrustRestrictionsPath: path.Join(
			volume.NodeCertificatesSecretVolumeMountPath,
			nodecerts.TrustRestrictionsFilename,
		),
	}

	return MustCanonicalConfig(cfg)
}
