// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

// Default values for the volume name and paths
const (
	ProbeUserSecretMountPath = "/mnt/elastic-internal/probe-user" //nolint:gosec
	ProbeUserVolumeName      = "elastic-internal-probe-user"

	ConfigVolumeMountPath               = "/usr/share/elasticsearch/config"
	NodeTransportCertificatePathSegment = "node-transport-cert"
	NodeTransportCertificateKeyFile     = "transport.tls.key"
	NodeTransportCertificateCertFile    = "transport.tls.crt"

	TransportCertificatesSecretVolumeName      = "elastic-internal-transport-certificates"
	TransportCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/transport-certs" //nolint:gosec

	RemoteCertificateAuthoritiesSecretVolumeName      = "elastic-internal-remote-certificate-authorities"
	RemoteCertificateAuthoritiesSecretVolumeMountPath = "/usr/share/elasticsearch/config/transport-remote-certs/" //nolint:gosec

	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/http-certs" //nolint:gosec

	XPackFileRealmVolumeName      = "elastic-internal-xpack-file-realm"
	XPackFileRealmVolumeMountPath = "/mnt/elastic-internal/xpack-file-realm"

	UnicastHostsVolumeName      = "elastic-internal-unicast-hosts"
	UnicastHostsVolumeMountPath = "/mnt/elastic-internal/unicast-hosts"
	UnicastHostsFile            = "unicast_hosts.txt"

	ElasticsearchDataVolumeName = "elasticsearch-data"
	ElasticsearchDataMountPath  = "/usr/share/elasticsearch/data"

	ElasticsearchLogsVolumeName = "elasticsearch-logs"
	ElasticsearchLogsMountPath  = "/usr/share/elasticsearch/logs"

	ScriptsVolumeName      = "elastic-internal-scripts"
	ScriptsVolumeMountPath = "/mnt/elastic-internal/scripts"

	DownwardAPIVolumeName = "downward-api"
	DownwardAPIMountPath  = "/mnt/elastic-internal/downward-api"
	LabelsFile            = "labels"
	AnnotationsFile       = "annotations"

	ServiceAccountsFile = "service_tokens"
)
