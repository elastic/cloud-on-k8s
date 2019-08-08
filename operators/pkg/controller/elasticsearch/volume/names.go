// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

// Default values for the volume name and paths
const (
	ProbeUserSecretMountPath = "/mnt/elastic-internal/probe-user"
	ProbeUserVolumeName      = "elastic-internal-probe-user"

	ConfigVolumeMountPath               = "/usr/share/elasticsearch/config"
	NodeTransportCertificatePathSegment = "node-transport-cert"
	NodeTransportCertificateKeyFile     = "transport.tls.key"
	NodeTransportCertificateCertFile    = "transport.tls.crt"

	TransportCertificatesSecretVolumeName      = "elastic-internal-transport-certificates"
	TransportCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/transport-certs"

	HTTPCertificatesSecretVolumeName      = "elastic-internal-http-certificates"
	HTTPCertificatesSecretVolumeMountPath = "/usr/share/elasticsearch/config/http-certs"

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
)
