// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	ESCertsPath     = "/mnt/elastic-internal/es-certs"
	ConfigMountPath = "/mnt/elastic-internal/config"
	ConfigFilename  = "enterprise-search.yml"

	SecretSessionSetting  = "secret_session_key"
	EncryptionKeysSetting = "secret_management.encryption_keys"
)

func ConfigSecretVolume(ents entsv1beta1.EnterpriseSearch) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(name.Config(ents.Name), "config", ConfigMountPath)
}

// Reconcile reconciles the configuration of Enterprise Search: it generates the right configuration and
// stores it in a secret that is kept up to date.
func ReconcileConfig(driver driver.Interface, ents entsv1beta1.EnterpriseSearch) (corev1.Secret, error) {
	cfg, err := newConfig(driver, ents)
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
			Namespace: ents.Namespace,
			Name:      name.Config(ents.Name),
			Labels:    common.AddCredentialsLabel(Labels(ents.Name)),
		},
		Data: map[string][]byte{
			ConfigFilename: cfgBytes,
		},
	}

	return reconciler.ReconcileSecret(driver.K8sClient(), expectedConfigSecret, &ents)
}

// newConfig builds a single merged config from:
// - ECK-managed default configuration
// - association configuration (eg. ES credentials)
// - TLS settings configuration
// - user-provided plaintext configuration
// - user-provided secret configuration
// In case of duplicate settings, the last one takes precedence.
func newConfig(driver driver.Interface, ents entsv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	reusedCfg, err := getOrCreateReusableSettings(driver.K8sClient(), ents)
	if err != nil {
		return nil, err
	}
	tlsCfg := tlsConfig(ents)
	associationCfg, err := associationConfig(driver.K8sClient(), ents)
	if err != nil {
		return nil, err
	}
	specConfig := ents.Spec.Config
	if specConfig == nil {
		specConfig = &commonv1.Config{}
	}
	userProvidedCfg, err := settings.NewCanonicalConfigFrom(specConfig.Data)
	if err != nil {
		return nil, err
	}
	userProvidedSecretCfg, err := parseConfigRef(driver, ents)
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig(ents)

	// merge with user settings last so they take precedence
	err = cfg.MergeWith(reusedCfg, tlsCfg, associationCfg, userProvidedCfg, userProvidedSecretCfg)
	return cfg, err
}

// reusableSettings captures secrets settings in the Enterprise Search configuration that we want to reuse.
type reusableSettings struct {
	SecretSession  string   `config:"secret_session_key"`
	EncryptionKeys []string `config:"secret_management.encryption_keys"`
}

// getOrCreateReusableSettings reads the current configuration and reuse existing secrets it they exist.
func getOrCreateReusableSettings(c k8s.Client, ents entsv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	cfg, err := getExistingConfig(c, ents)
	if err != nil {
		return nil, err
	}

	var e reusableSettings
	if cfg == nil {
		e = reusableSettings{}
	} else if err := cfg.Unpack(&e); err != nil {
		return nil, err
	}
	if len(e.SecretSession) == 0 {
		e.SecretSession = string(common.RandomBytes(32))
	}
	if len(e.EncryptionKeys) == 0 {
		e.EncryptionKeys = []string{string(common.RandomBytes(32))}
	}
	return settings.MustCanonicalConfig(e), nil
}

// getExistingConfig retrieves the canonical config, if one exists
func getExistingConfig(client k8s.Client, ents entsv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: ents.Namespace,
		Name:      name.Config(ents.Name),
	}
	err := client.Get(key, &secret)
	if err != nil && apierrors.IsNotFound(err) {
		log.V(1).Info("Enterprise Search config secret does not exist", "namespace", ents.Namespace, "ents_name", ents.Name)
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	rawCfg, exists := secret.Data[ConfigFilename]
	if !exists {
		return nil, nil
	}
	cfg, err := settings.ParseConfig(rawCfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// configRefWatchName returns the name of the watch registered on Kubernetes secrets referenced in `configRef`.
func configRefWatchName(ents types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-configref", ents.Namespace, ents.Name)
}

// parseConfigRef builds a single merged CanonicalConfig from the secrets referenced in configRef,
// and ensures watches are correctly set on those secrets.
func parseConfigRef(driver driver.Interface, ents entsv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	cfg := settings.NewCanonicalConfig()
	secretNames := make([]string, 0, len(ents.Spec.ConfigRef))
	for _, secretRef := range ents.Spec.ConfigRef {
		if secretRef.SecretName == "" {
			continue
		}
		secretNames = append(secretNames, secretRef.SecretName)
	}
	nsn := k8s.ExtractNamespacedName(&ents)
	if err := watches.WatchUserProvidedSecrets(nsn, driver.DynamicWatches(), configRefWatchName(nsn), secretNames); err != nil {
		return nil, err
	}
	for _, secretName := range secretNames {
		var secret corev1.Secret
		if err := driver.K8sClient().Get(types.NamespacedName{Namespace: ents.Namespace, Name: secretName}, &secret); err != nil {
			// the secret may not exist (yet) in the cache
			// it may contain important settings such as encryption keys, that we don't want to generate ourselves
			// let's explicitly error out
			return nil, err
		}
		if data, exists := secret.Data[ConfigFilename]; exists {
			parsed, err := settings.ParseConfig(data)
			if err != nil {
				msg := "unable to parse configuration from secret"
				log.Error(err, msg, "namespace", ents.Namespace, "ents_name", ents.Name, "secret_name", secretName)
				driver.Recorder().Event(&ents, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+secretName)
				return nil, err
			}
			if err := cfg.MergeWith(parsed); err != nil {
				return nil, err
			}
		}
	}
	return cfg, nil
}

func defaultConfig(ents entsv1beta1.EnterpriseSearch) *settings.CanonicalConfig {
	return settings.MustCanonicalConfig(map[string]interface{}{
		"ent_search.external_url":        fmt.Sprintf("%s://localhost:%d", ents.Spec.HTTP.Protocol(), HTTPPort),
		"ent_search.listen_host":         "0.0.0.0",
		"allow_es_settings_modification": true,
	})
}

func associationConfig(c k8s.Client, ents entsv1beta1.EnterpriseSearch) (*settings.CanonicalConfig, error) {
	if !ents.AssociationConf().IsConfigured() {
		return settings.NewCanonicalConfig(), nil
	}

	username, password, err := association.ElasticsearchAuthSettings(c, &ents)
	if err != nil {
		return nil, err
	}
	cfg := settings.MustCanonicalConfig(map[string]string{
		"ent_search.auth.source": "elasticsearch-native",
		"elasticsearch.host":     ents.AssociationConf().URL,
		"elasticsearch.username": username,
		"elasticsearch.password": password,
	})

	if ents.AssociationConf().CAIsConfigured() {
		if err := cfg.MergeWith(settings.MustCanonicalConfig(map[string]interface{}{
			"elasticsearch.ssl.enabled":               true,
			"elasticsearch.ssl.certificate_authority": filepath.Join(ESCertsPath, certificates.CertFileName),
		})); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func tlsConfig(ents entsv1beta1.EnterpriseSearch) *settings.CanonicalConfig {
	if !ents.Spec.HTTP.TLS.Enabled() {
		return settings.NewCanonicalConfig()
	}
	certsDir := certificates.HTTPCertSecretVolume(name.EntSearchNamer, ents.Name).VolumeMount().MountPath
	return settings.MustCanonicalConfig(map[string]interface{}{
		"ent_search.ssl.enabled":                 true,
		"ent_search.ssl.certificate":             filepath.Join(certsDir, certificates.CertFileName),
		"ent_search.ssl.key":                     filepath.Join(certsDir, certificates.KeyFileName),
		"ent_search.ssl.certificate_authorities": []string{filepath.Join(certsDir, certificates.CAFileName)},
	})
}
