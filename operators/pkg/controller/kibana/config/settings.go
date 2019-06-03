// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/escerts"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

// Kibana configuration settings file
const settingsFilename = "kibana.yml"

// NewConfigSettings returns the Kibana configuration settings for the given Kibana resource.
func NewConfigSettings(client k8s.Client, kb v1alpha1.Kibana) (*settings.CanonicalConfig, error) {
	kbSettings := settings.NewCanonicalConfig()

	esAuthSettings, err := elasticsearchAuthSettings(client, kb)
	if err != nil {
		return nil, err
	}

	err = kbSettings.MergeWith(
		settings.MustCanonicalConfig(baseSettings(kb)),
		settings.MustCanonicalConfig(elasticsearchTLSSettings(kb)),
		settings.MustCanonicalConfig(esAuthSettings),
	)
	if err != nil {
		return nil, err
	}

	return kbSettings, nil
}

func baseSettings(kb v1alpha1.Kibana) map[string]interface{} {
	return map[string]interface{}{
		ServerName:         kb.Name,
		ServerHost:         "0",
		ElasticSearchHosts: []string{kb.Spec.Elasticsearch.URL},
		XpackMonitoringUiContainerElasticsearchEnabled: true,
	}
}
func elasticsearchTLSSettings(kb v1alpha1.Kibana) map[string]interface{} {
	esCertsVolumeMountPath := escerts.SecretVolume(kb).VolumeMount().MountPath
	return map[string]interface{}{
		ElasticsearchSslCertificateAuthorities: path.Join(esCertsVolumeMountPath, certificates.CAFileName),
		ElasticsearchSslVerificationMode:       "certificate",
	}
}

func elasticsearchAuthSettings(client k8s.Client, kb v1alpha1.Kibana) (map[string]interface{}, error) {
	authSettings := map[string]interface{}{}
	auth := kb.Spec.Elasticsearch.Auth
	if auth.Inline != nil {
		authSettings = map[string]interface{}{
			ElasticsearchUsername: auth.Inline.Username,
			ElasticsearchPassword: auth.Inline.Password,
		}
	}
	if auth.SecretKeyRef != nil {
		secret, err := getConfigSecret(client, kb)
		if err != nil {
			return nil, err
		}
		authSettings = map[string]interface{}{
			ElasticsearchUsername: auth.SecretKeyRef.Key,
			ElasticsearchPassword: string(secret.Data[auth.SecretKeyRef.Key]),
		}
	}
	return authSettings, nil
}
