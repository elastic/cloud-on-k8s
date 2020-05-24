// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	defaultResources = corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("200Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}
)

// setOutput will set output section in Beat config according to association configuration.
func setOutput(cfg *settings.CanonicalConfig, client k8s.Client, associated commonv1.Associated) error {
	if associated.AssociationConf().IsConfigured() {
		username, password, err := association.ElasticsearchAuthSettings(client, associated)
		if err != nil {
			return err
		}

		if err := cfg.MergeWith(settings.MustCanonicalConfig(
			map[string]interface{}{
				"output.elasticsearch": map[string]interface{}{
					"hosts":    []string{associated.AssociationConf().GetURL()},
					"username": username,
					"password": password,
				},
			})); err != nil {
			return err
		}

		if associated.AssociationConf().GetCACertProvided() {
			if err := cfg.MergeWith(settings.MustCanonicalConfig(
				map[string]interface{}{
					"output.elasticsearch.ssl.certificate_authorities": path.Join(CAMountPath, CAFileName),
				})); err != nil {
				return err
			}
		}
	}

	return nil
}

func buildBeatConfig(
	client k8s.Client,
	associated commonv1.Associated,
	defaultConfig *settings.CanonicalConfig,
	userConfig *commonv1.Config) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	if defaultConfig == nil && userConfig == nil {
		return nil, fmt.Errorf("both default and user configs are nil")
	}

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

func reconcileConfig(
	params DriverParams,
	defaultConfig *settings.CanonicalConfig,
	checksum hash.Hash) error {

	cfgBytes, err := buildBeatConfig(params.Client, params.Associated, defaultConfig, params.Config)
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
			ConfigFileName: cfgBytes,
		},
	}

	// reconcile
	if _, err = reconciler.ReconcileSecret(params.Client, expected, params.Owner); err != nil {
		return err
	}

	// we need to deref the secret here (if any) to include it in the checksum otherwise Beat will not be rolled on content changes
	if err := commonassociation.WriteAssocSecretToHash(params.Client, params.Associated, checksum); err != nil {
		return err
	}

	_, _ = checksum.Write(cfgBytes)

	return nil
}
