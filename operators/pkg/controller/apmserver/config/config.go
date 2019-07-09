// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"fmt"
	"path/filepath"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

const (
	// DefaultHTTPPort is the (default) port used by ApmServer
	DefaultHTTPPort = 8200

	// Certificates
	CertificatesDir = "config/elasticsearch-certs"
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
	if as.Spec.Output.Elasticsearch.IsConfigured() {
		// Get username and password
		username, password, err := association.ElasticsearchAuthSettings(c, &as)
		if err != nil {
			return nil, err
		}
		outputCfg = settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch.hosts":                       as.Spec.Output.Elasticsearch.Hosts,
				"output.elasticsearch.username":                    username,
				"output.elasticsearch.password":                    password,
				"output.elasticsearch.ssl.certificate_authorities": []string{filepath.Join(CertificatesDir, certificates.CertFileName)},
			},
		)

	}

	// Create a base configuration.
	cfg := settings.MustCanonicalConfig(map[string]interface{}{
		"apm-server.host":         fmt.Sprintf(":%d", DefaultHTTPPort),
		"apm-server.secret_token": "${SECRET_TOKEN}",
	})

	// Merge the configuration with userSettings last so they take precedence.
	err = cfg.MergeWith(
		outputCfg,
		userSettings,
	)

	if err != nil {
		return nil, err
	}
	return cfg, nil
}
