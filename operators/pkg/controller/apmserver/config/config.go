// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// DefaultHTTPPort is the (default) port used by ApmServer
const DefaultHTTPPort = 8200

// FromResourceSpec resolves the ApmServer configuration to use based on the provided spec.
// TODO: missing test
func FromResourceSpec(c k8s.Client, as v1alpha1.ApmServer) (*Config, error) {
	// TODO: consider scaling the default values provided based on the apm server resources
	// these defaults are taken (without scaling) from a defaulted ECE install

	username, password, err := getCredentials(c, as)
	if err != nil {
		return nil, err
	}

	return &Config{
		Name: "${POD_NAME}",
		ApmServer: ApmServerConfig{
			Host:               fmt.Sprintf(":%d", DefaultHTTPPort),
			SecretToken:        "${SECRET_TOKEN}",
			ReadTimeout:        3600,
			ShutdownTimeout:    "30s",
			Rum:                RumConfig{Enabled: true, RateLimit: 10},
			ConcurrentRequests: 1,
			MaxUnzippedSize:    5242880,
			// TODO: TLS support for the server itself
			SSL: TLSConfig{
				Enabled: false,
			},
		},
		XPackMonitoringEnabled: true,

		Logging: LoggingConfig{
			JSON:           true,
			MetricsEnabled: true,
		},
		Queue: QueueConfig{
			Mem: QueueMemConfig{
				Events: 2000,
				Flush: FlushConfig{
					MinEvents: 267,
					Timeout:   "1s",
				},
			},
		},
		SetupTemplateSettingsIndex: SetupTemplateSettingsIndex{
			NumberOfShards:     1,
			NumberOfReplicas:   1,
			AutoExpandReplicas: "0-2",
		},
		Output: OutputConfig{
			Elasticsearch: ElasticsearchOutputConfig{
				Worker:           5,
				MaxBulkSize:      267,
				CompressionLevel: 5,
				Hosts:            as.Spec.Output.Elasticsearch.Hosts,
				Username:         username,
				Password:         password,
				// TODO: optional TLS
				SSL: TLSConfig{
					Enabled: true,
					// TODO: hardcoded path
					CertificateAuthorities: []string{"config/elasticsearch-certs/" + certificates.CAFileName},
				},
				// TODO: include indices? or will they be defaulted fine?
			},
		},
	}, nil
}

func getCredentials(c k8s.Client, as v1alpha1.ApmServer) (username, password string, err error) {
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

type Config struct {
	Name                       string                     `json:"name,omitempty"`
	ApmServer                  ApmServerConfig            `json:"apm-server,omitempty"`
	XPackMonitoringEnabled     bool                       `json:"xpack.monitoring.enabled,omitempty"`
	Logging                    LoggingConfig              `json:"logging,omitempty"`
	Queue                      QueueConfig                `json:"queue,omitempty"`
	Output                     OutputConfig               `json:"output,omitempty"`
	SetupTemplateSettingsIndex SetupTemplateSettingsIndex `json:"setup.template.settings.index,omitempty"`
}

type OutputConfig struct {
	Elasticsearch ElasticsearchOutputConfig `json:"elasticsearch,omitempty"`
	// TODO support other outputs.
}

type SetupTemplateSettingsIndex struct {
	NumberOfShards     int    `json:"number_of_shards,omitempty"`
	NumberOfReplicas   int    `json:"number_of_replicas,omitempty"`
	AutoExpandReplicas string `json:"auto_expand_replicas,omitempty"`
}

type ApmServerConfig struct {
	Host               string    `json:"host,omitempty"`
	ReadTimeout        int       `json:"read_timeout,omitempty"`
	ShutdownTimeout    string    `json:"shutdown_timeout,omitempty"`
	SecretToken        string    `json:"secret_token,omitempty"`
	SSL                TLSConfig `json:"ssl,omitempty"`
	Rum                RumConfig `json:"rum,omitempty"`
	ConcurrentRequests int       `json:"concurrent_requests,omitempty"`
	MaxUnzippedSize    int       `json:"max_unzipped_size,omitempty"`
}

type RumConfig struct {
	Enabled   bool `json:"enabled,omitempty"`
	RateLimit int  `json:"rate_limit,omitempty"`
}

type TLSConfig struct {
	Enabled                bool     `json:"enabled"`
	Certificate            string   `json:"certificate,omitempty"`
	Key                    string   `json:"key,omitempty"`
	CertificateAuthorities []string `json:"certificate_authorities,omitempty"`
}

type LoggingConfig struct {
	Level          string `json:"level,omitempty"`
	ToFiles        bool   `json:"to_files,omitempty"`
	JSON           bool   `json:"json,omitempty"`
	MetricsEnabled bool   `json:"metrics.enabled,omitempty"`
}

type LoggingFilesConfig struct {
	Path      string `json:"path,omitempty"`
	Name      string `json:"name,omitempty"`
	Keepfiles int    `json:"keepfiles,omitempty"`
}

type LoggingMetricsConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type QueueConfig struct {
	Mem QueueMemConfig `json:"mem,omitempty"`
}

type QueueMemConfig struct {
	Events int         `json:"events,omitempty"`
	Flush  FlushConfig `json:"flush,omitempty"`
}

type FlushConfig struct {
	MinEvents int    `json:"min_events,omitempty"`
	Timeout   string `json:"timeout,omitempty"`
}

type ElasticsearchOutputConfig struct {
	Hosts            []string          `json:"hosts,omitempty"`
	SSL              TLSConfig         `json:"ssl,omitempty"`
	Username         string            `json:"username,omitempty"`
	Password         string            `json:"password,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Worker           int               `json:"worker,omitempty"`
	MaxBulkSize      int               `json:"max_bulk_size,omitempty"`
	CompressionLevel int               `json:"compression_level,omitempty"`
	Indices          []json.RawMessage `json:"indices,omitempty"`
}
