// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	"context"
	"hash"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	filebeatDefaultConfig = settings.MustCanonicalConfig(map[string]interface{}{
		"filebeat": map[string]interface{}{
			"autodiscover": map[string]interface{}{
				"providers": []map[string]interface{}{
					{
						"type": "kubernetes",
						"node": "${NODE_NAME}",
						"hints": map[string]interface{}{
							"enabled": "true",
							"default_config": map[string]interface{}{
								"type":  "container",
								"paths": []string{"/var/log/containers/*${data.kubernetes.container.id}.log"},
							},
						},
					},
				},
			},
		},
		"processors": []map[string]interface{}{
			{"add_cloud_metadata": nil},
			{"add_host_metadata": nil},
		},
	})
)

func build(client k8s.Client, associated commonv1.Associated, config *commonv1.Config) ([]byte, error) {
	cfg := settings.NewCanonicalConfig()

	if err := commonbeat.SetOutput(cfg, client, associated); err != nil {
		return nil, err
	}

	// use only the default config or only the provided config - no overriding, no merging
	if config == nil {
		if err := cfg.MergeWith(filebeatDefaultConfig); err != nil {
			return nil, err
		}
	} else {
		userCfg, err := settings.NewCanonicalConfigFrom(config.Data)
		if err != nil {
			return nil, err
		}

		if err = cfg.MergeWith(userCfg); err != nil {
			return nil, err
		}
	}

	return cfg.Render()
}

// ReconcileConfig builds and reconciles Filebeat config based on association configured, default and user configs.
// `checksum` hash will get updated based on the config to be consumed by Filebeat.
func ReconcileConfig(
	ctx context.Context,
	client k8s.Client,
	associated commonv1.Associated,
	cfg *commonv1.Config,
	owner metav1.Object,
	labels map[string]string,
	namer commonbeat.Namer,
	checksum hash.Hash) error {
	span, _ := apm.StartSpan(ctx, "reconcile_config_secret", tracing.SpanTypeApp)
	defer span.End()

	// build config
	cfgBytes, err := build(client, associated, cfg)
	if err != nil {
		return err
	}

	// create resource
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: owner.GetNamespace(),
			Name:      namer.ConfigSecretName(string(Type), owner.GetName()),
			Labels:    common.AddCredentialsLabel(labels),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	// reconcile
	if _, err = reconciler.ReconcileSecret(client, expected, owner); err != nil {
		return err
	}

	// we need to deref the secret here (if any) to include it in the checksum otherwise Beat will not be rolled on contents changes
	assocConf := associated.AssociationConf()
	if assocConf.AuthIsConfigured() {
		esAuthKey := types.NamespacedName{Name: assocConf.GetAuthSecretName(), Namespace: owner.GetNamespace()}
		esAuthSecret := corev1.Secret{}
		if err := client.Get(esAuthKey, &esAuthSecret); err != nil {
			return err
		}
		_, _ = checksum.Write(esAuthSecret.Data[assocConf.GetAuthSecretKey()])
	}

	_, _ = checksum.Write(cfgBytes)

	return nil
}
