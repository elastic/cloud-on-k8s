// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	CAMountPath = "/mnt/elastic-internal/es-certs/"
	CAFileName  = "ca.crt"

	// ConfigChecksumLabel is a label used to store beats config checksum.
	ConfigChecksumLabel = "beat.k8s.elastic.co/config-checksum"

	// VersionLabelName is a label used to track the version of a Beat Pod.
	VersionLabelName = "beat.k8s.elastic.co/version"
)

// SetOutput will set output section in Beat config according to association configuration.
func setOutput(cfg *settings.CanonicalConfig, client k8s.Client, associated commonv1.Associated) error {
	if associated.AssociationConf().IsConfigured() {
		username, password, err := association.ElasticsearchAuthSettings(client, associated)
		if err != nil {
			return err
		}

		return cfg.MergeWith(settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch": map[string]interface{}{
					"hosts":                       []string{associated.AssociationConf().GetURL()},
					"username":                    username,
					"password":                    password,
					"ssl.certificate_authorities": path.Join(CAMountPath, CAFileName),
				},
			}))
	}

	return nil
}

func build(
	client k8s.Client,
	associated commonv1.Associated,
	defaultConfig *settings.CanonicalConfig,
	userConfig *commonv1.Config) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	if err := setOutput(cfg, client, associated); err != nil {
		return nil, err
	}

	// use only the default config or only the provided config - no overriding, no merging
	if userConfig == nil {
		if err := cfg.MergeWith(defaultConfig); err != nil {
			return nil, err
		}
	} else {
		userCfg, err := settings.NewCanonicalConfigFrom(userConfig.Data)
		if err != nil {
			return nil, err
		}

		if err = cfg.MergeWith(userCfg); err != nil {
			return nil, err
		}
	}

	return cfg.Render()
}

func ReconcileConfig(
	params DriverParams,
	configFileName string,
	defaultConfig *settings.CanonicalConfig,
	checksum hash.Hash) error {

	cfgBytes, err := build(params.Client, params.Associated, defaultConfig, params.Config)
	if err != nil {
		return err
	}

	// create resource
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Owner.GetNamespace(),
			Name:      params.Namer.ConfigSecretName(params.Type, params.Owner.GetName()),
			Labels:    common.AddCredentialsLabel(params.Labels),
		},
		Data: map[string][]byte{
			configFileName: cfgBytes,
		},
	}

	// reconcile
	if _, err = reconciler.ReconcileSecret(params.Client, expected, params.Owner); err != nil {
		return err
	}

	// we need to deref the secret here (if any) to include it in the checksum otherwise Beat will not be rolled on contents changes
	assocConf := params.Associated.AssociationConf()
	if assocConf.AuthIsConfigured() {
		esAuthKey := types.NamespacedName{Name: assocConf.GetAuthSecretName(), Namespace: params.Owner.GetNamespace()}
		esAuthSecret := corev1.Secret{}
		if err := params.Client.Get(esAuthKey, &esAuthSecret); err != nil {
			return err
		}
		_, _ = checksum.Write(esAuthSecret.Data[assocConf.GetAuthSecretKey()])
	}

	_, _ = checksum.Write(cfgBytes)

	return nil
}
