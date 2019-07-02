// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

// Constants to use for the Kibana configuration settings.
const (
	ServerName                                     = "server.name"
	ServerHost                                     = "server.host"
	ElasticSearchHosts                             = "elasticsearch.hosts"
	XpackMonitoringUiContainerElasticsearchEnabled = "xpack.monitoring.ui.container.elasticsearch.enabled"

	ElasticsearchSslCertificateAuthorities = "elasticsearch.ssl.certificateAuthorities"
	ElasticsearchSslVerificationMode       = "elasticsearch.ssl.verificationMode"

	ElasticsearchUsername = "elasticsearch.username"
	ElasticsearchPassword = "elasticsearch.password"

	ElasticsearchURL   = "elasticsearch.url"
	ElasticsearchHosts = "elasticsearch.hosts"

	ServerSSLEnabled     = "server.ssl.enabled"
	ServerSSLCertificate = "server.ssl.certificate"
	ServerSSLKey         = "server.ssl.key"
)
