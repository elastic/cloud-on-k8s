// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"path"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

const (
	ESCertsPath     = "/mnt/elastic-internal/es-certs"
	ConfigFilename  = "elastic-maps-server.yml"
	ConfigMountPath = "/usr/src/app/server/config/elastic-maps-server.yml"
)

func configSecretVolume(ems emsv1alpha1.ElasticMapsServer) volume.SecretVolume {
	return volume.NewSecretVolume(Config(ems.Name), "config", ConfigMountPath, ConfigFilename, 0444)
}

func reconcileConfig(driver driver.Interface, ems emsv1alpha1.ElasticMapsServer, ipFamily corev1.IPFamily) (corev1.Secret, error) {
	cfg, err := newConfig(driver, ems, ipFamily)
	if err != nil {
		return corev1.Secret{}, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return corev1.Secret{}, err
	}

	// Reconcile the configuration in a secret
	expectedConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ems.Namespace,
			Name:      Config(ems.Name),
			Labels:    common.AddCredentialsLabel(labels(ems.Name)),
		},
		Data: map[string][]byte{
			ConfigFilename: cfgBytes,
		},
	}

	return reconciler.ReconcileSecret(driver.K8sClient(), expectedConfigSecret, &ems)
}

func newConfig(d driver.Interface, ems emsv1alpha1.ElasticMapsServer, ipFamily corev1.IPFamily) (*settings.CanonicalConfig, error) {
	cfg := settings.NewCanonicalConfig()

	inlineUserCfg, err := inlineUserConfig(ems.Spec.Config)
	if err != nil {
		return cfg, err
	}

	refUserCfg, err := common.ParseConfigRef(d, &ems, ems.Spec.ConfigRef, ConfigFilename)
	if err != nil {
		return cfg, err
	}

	defaults := defaultConfig(ipFamily)
	tls := tlsConfig(ems)
	assocCfg, err := associationConfig(d.K8sClient(), ems)
	if err != nil {
		return cfg, err
	}
	err = cfg.MergeWith(inlineUserCfg, refUserCfg, defaults, tls, assocCfg)
	return cfg, err
}

func inlineUserConfig(cfg *commonv1.Config) (*settings.CanonicalConfig, error) {
	if cfg == nil {
		cfg = &commonv1.Config{}
	}
	return settings.NewCanonicalConfigFrom(cfg.Data)
}

func defaultConfig(ipFamily corev1.IPFamily) *settings.CanonicalConfig {
	return settings.MustCanonicalConfig(map[string]interface{}{
		"host": net.InAddrAnyFor(ipFamily).String(),
	})
}

func tlsConfig(ems emsv1alpha1.ElasticMapsServer) *settings.CanonicalConfig {
	if !ems.Spec.HTTP.TLS.Enabled() {
		return settings.NewCanonicalConfig()
	}
	return settings.MustCanonicalConfig(map[string]interface{}{
		"ssl.enabled":     true,
		"ssl.certificate": path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.CertFileName),
		"ssl.key":         path.Join(certificates.HTTPCertificatesSecretVolumeMountPath, certificates.KeyFileName),
	})
}

func associationConfig(c k8s.Client, ems emsv1alpha1.ElasticMapsServer) (*settings.CanonicalConfig, error) {
	cfg := settings.NewCanonicalConfig()
	assocConf, err := ems.AssociationConf()
	if err != nil {
		return nil, err
	}
	if !assocConf.IsConfigured() {
		return cfg, nil
	}
	credentials, err := association.ElasticsearchAuthSettings(c, &ems)
	if err != nil {
		return nil, err
	}
	if err := cfg.MergeWith(settings.MustCanonicalConfig(map[string]string{
		"elasticsearch.host":     assocConf.URL,
		"elasticsearch.username": credentials.Username,
		"elasticsearch.password": credentials.Password,
	})); err != nil {
		return nil, err
	}

	if assocConf.GetCACertProvided() {
		if err := cfg.MergeWith(settings.MustCanonicalConfig(map[string]interface{}{
			"elasticsearch.ssl.verificationMode":       "certificate",
			"elasticsearch.ssl.certificateAuthorities": filepath.Join(ESCertsPath, certificates.CAFileName),
		})); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
