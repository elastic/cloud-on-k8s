// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// DefaultHTTPPort is the (default) port used by ApmServer
	DefaultHTTPPort = 8200

	// Certificates
	CertificatesDir = "config/elasticsearch-certs"

	APMServerHost        = "apm-server.host"
	APMServerSecretToken = "apm-server.secret_token"

	APMServerSSLEnabled     = "apm-server.ssl.enabled"
	APMServerSSLKey         = "apm-server.ssl.key"
	APMServerSSLCertificate = "apm-server.ssl.certificate"
)

func NewConfigFromSpec(c k8s.Client, as v1alpha1.ApmServer) (*settings.CanonicalConfig, error) {
	specConfig := as.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1alpha1.Config{}
	}

	userSettings, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return nil, err
	}

	outputCfg := settings.NewCanonicalConfig()
	if as.Spec.Elasticsearch.IsConfigured() {
		// Get username and password
		username, password, err := association.ElasticsearchAuthSettings(c, &as)
		if err != nil {
			return nil, err
		}
		outputCfg = settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch.hosts":                       as.Spec.Elasticsearch.Hosts,
				"output.elasticsearch.username":                    username,
				"output.elasticsearch.password":                    password,
				"output.elasticsearch.ssl.certificate_authorities": []string{filepath.Join(CertificatesDir, certificates.CertFileName)},
			},
		)

	}

	// Create a base configuration.

	cfg := settings.MustCanonicalConfig(map[string]interface{}{
		APMServerHost:        fmt.Sprintf(":%d", DefaultHTTPPort),
		APMServerSecretToken: "${SECRET_TOKEN}",
	})

	// Merge the configuration with userSettings last so they take precedence.
	err = cfg.MergeWith(
		outputCfg,
		settings.MustCanonicalConfig(tlsSettings(as)),
		userSettings,
	)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func tlsSettings(as v1alpha1.ApmServer) map[string]interface{} {
	if !as.Spec.HTTP.TLS.Enabled() {
		return nil
	}
	return map[string]interface{}{
		APMServerSSLEnabled:     true,
		APMServerSSLCertificate: path.Join(http.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),
		APMServerSSLKey:         path.Join(http.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
	}

}
