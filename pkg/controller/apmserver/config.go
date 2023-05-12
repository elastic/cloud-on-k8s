// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"fmt"
	"path"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	// DefaultHTTPPort is the (default) port used by ApmServer
	DefaultHTTPPort = 8200

	APMServerHost              = "apm-server.host"
	APMServerLegacySecretToken = "apm-server.secret_token"      //nolint:gosec
	APMServerSecretToken       = "apm-server.auth.secret_token" //nolint:gosec

	APMServerSSLEnabled     = "apm-server.ssl.enabled"
	APMServerSSLKey         = "apm-server.ssl.key"
	APMServerSSLCertificate = "apm-server.ssl.certificate"

	ApmCfgSecretKey = "apm-server.yml" //nolint:gosec
)

func certificatesDir(associationType commonv1.AssociationType) string {
	return fmt.Sprintf("config/%s-certs", associationType)
}

func apmServerSecretTokenKeyFor(v version.Version) string {
	if v.GTE(version.MinFor(8, 0, 0)) {
		return APMServerSecretToken
	}
	return APMServerLegacySecretToken
}

// reconcileApmServerConfig reconciles the configuration of the APM server: it first creates the configuration from the APM
// specification and then reconcile the underlying secret.
func reconcileApmServerConfig(ctx context.Context, client k8s.Client, as *apmv1.ApmServer, version version.Version) (corev1.Secret, error) {
	// Create a new configuration from the APM object spec.
	cfg, err := newConfigFromSpec(ctx, client, as, version)
	if err != nil {
		return corev1.Secret{}, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return corev1.Secret{}, err
	}

	// reconcile the configuration in a secret
	expectedConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      Config(as.Name),
			Labels:    as.GetIdentityLabels(),
		},
		Data: map[string][]byte{
			ApmCfgSecretKey: cfgBytes,
		},
	}
	return reconciler.ReconcileSecret(ctx, client, expectedConfigSecret, as)
}

func newConfigFromSpec(ctx context.Context, c k8s.Client, as *apmv1.ApmServer, version version.Version) (*settings.CanonicalConfig, error) {
	cfg := settings.MustCanonicalConfig(map[string]interface{}{
		APMServerHost:                       fmt.Sprintf(":%d", DefaultHTTPPort),
		apmServerSecretTokenKeyFor(version): "${SECRET_TOKEN}",
	})

	esConfig, err := newElasticsearchConfigFromSpec(ctx, c, apmv1.ApmEsAssociation{ApmServer: as})
	if err != nil {
		return nil, err
	}

	kibanaConfig, err := newKibanaConfigFromSpec(ctx, c, apmv1.ApmKibanaAssociation{ApmServer: as})
	if err != nil {
		return nil, err
	}

	var userSettings *settings.CanonicalConfig
	if as.Spec.Config != nil {
		if userSettings, err = settings.NewCanonicalConfigFrom(as.Spec.Config.Data); err != nil {
			return nil, err
		}
	}

	// Merge the configuration with userSettings last so they take precedence.
	err = cfg.MergeWith(
		esConfig,
		kibanaConfig,
		settings.MustCanonicalConfig(tlsSettings(as)),
		userSettings,
	)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func newElasticsearchConfigFromSpec(ctx context.Context, c k8s.Client, esAssociation apmv1.ApmEsAssociation) (*settings.CanonicalConfig, error) {
	esAssocConf, err := esAssociation.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !esAssocConf.IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	// Get username and password
	credentials, err := association.ElasticsearchAuthSettings(ctx, c, &esAssociation)
	if err != nil {
		return nil, err
	}

	tmpOutputCfg := map[string]interface{}{
		"output.elasticsearch.hosts":    []string{esAssocConf.GetURL()},
		"output.elasticsearch.username": credentials.Username,
		"output.elasticsearch.password": credentials.Password,
	}
	if esAssocConf.GetCACertProvided() {
		tmpOutputCfg["output.elasticsearch.ssl.certificate_authorities"] = []string{filepath.Join(certificatesDir(esAssociation.AssociationType()), certificates.CAFileName)}
	}

	return settings.MustCanonicalConfig(tmpOutputCfg), nil
}

func newKibanaConfigFromSpec(ctx context.Context, c k8s.Client, kibanaAssociation apmv1.ApmKibanaAssociation) (*settings.CanonicalConfig, error) {
	kbAssocConf, err := kibanaAssociation.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !kbAssocConf.IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	// Get username and password
	credentials, err := association.ElasticsearchAuthSettings(ctx, c, &kibanaAssociation)
	if err != nil {
		return nil, err
	}

	tmpOutputCfg := map[string]interface{}{
		"apm-server.kibana.enabled":  true,
		"apm-server.kibana.host":     kbAssocConf.GetURL(),
		"apm-server.kibana.username": credentials.Username,
		"apm-server.kibana.password": credentials.Password,
	}
	if kbAssocConf.GetCACertProvided() {
		tmpOutputCfg["apm-server.kibana.ssl.certificate_authorities"] = []string{filepath.Join(certificatesDir(kibanaAssociation.AssociationType()), certificates.CAFileName)}
	}

	return settings.MustCanonicalConfig(tmpOutputCfg), nil
}

func tlsSettings(as *apmv1.ApmServer) map[string]interface{} {
	if !as.Spec.HTTP.TLS.Enabled() {
		return nil
	}
	return map[string]interface{}{
		APMServerSSLEnabled:     true,
		APMServerSSLCertificate: path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),
		APMServerSSLKey:         path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
	}
}
