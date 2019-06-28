// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// DefaultHTTPPort is the (default) port used by ApmServer
	DefaultHTTPPort = 8200
)

var DefaultConfiguration = []byte(`
apm-server:
  concurrent_requests: 1
  max_unzipped_size: 5242880
  read_timeout: 3600
  rum:
    enabled: true
    rate_limit: 10
  shutdown_timeout: 30s
  ssl:
    enabled: false
logging:
  json: true
  metrics.enabled: true
output:
  elasticsearch:
    compression_level: 5
    max_bulk_size: 267
    worker: 5
queue:
  mem:
    events: 2000
    flush:
      min_events: 267
      timeout: 1s
setup.template.settings.index:
  auto_expand_replicas: 0-2
  number_of_replicas: 1
  number_of_shards: 1
xpack.monitoring.enabled: true
`)

func NewConfigFromSpec(c k8s.Client, as v1alpha1.ApmServer) (*settings.CanonicalConfig, error) {
	specConfig := as.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1alpha1.Config{}
	}

	userSettings, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return nil, err
	}

	userSettings.

	// Get username and password
	username, password, err := elasticsearchAuthSettings(c, as)
	if err != nil {
		return nil, err
	}

	// Create a base configuration.
	cfg := settings.MustCanonicalConfig(map[string]interface{}{
		"apm-server.host":         fmt.Sprintf(":%d", DefaultHTTPPort),
		"apm-server.secret_token": "${SECRET_TOKEN}",
	})

	// Build the default configuration
	defaultCfg, err := settings.ParseConfig(DefaultConfiguration)
	if err != nil {
		return nil, err
	}

	// Merge the configuration with userSettings last so they take precedence.
	err = cfg.MergeWith(
		defaultCfg,
		settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch.hosts":                       as.Spec.Output.Elasticsearch.Hosts,
				"output.elasticsearch.username":                    username,
				"output.elasticsearch.password":                    password,
				"output.elasticsearch.ssl.certificate_authorities": []string{"config/elasticsearch-certs/" + certificates.CertFileName},
			},
		),
		userSettings,
	)
	return cfg, nil
}

func elasticsearchAuthSettings(c k8s.Client, as v1alpha1.ApmServer) (username, password string, err error) {
	auth := as.Spec.Output.Elasticsearch.Auth

	if auth.Inline != nil {
		return auth.Inline.Username, auth.Inline.Password, nil
	}

	// if auth is provided via a secret, resolve credentials from it.
	if auth.SecretKeyRef != nil {
		secretObjKey := types.NamespacedName{Namespace: as.Namespace, Name: auth.SecretKeyRef.Name}
		var secret v1.Secret
		if err := c.Get(secretObjKey, &secret); err != nil {
			return "", "", err
		}
		return auth.SecretKeyRef.Key, string(secret.Data[auth.SecretKeyRef.Key]), nil
	}

	// no authentication method provided, return an empty credential
	return "", "", nil
}
