// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"hash"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func reconcileConfig(params Params, configHash hash.Hash) *reconciler.Results {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	cfgBytes, err := buildConfig(params)
	if err != nil {
		return results.WithError(err)
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Logstash.Namespace,
			Name:      logstashv1alpha1.ConfigSecretName(params.Logstash.Name),
			Labels:    labels.AddCredentialsLabel(params.Logstash.GetIdentityLabels()),
		},
		Data: map[string][]byte{
			LogstashConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Context, params.Client, expected, &params.Logstash); err != nil {
		return results.WithError(err)
	}

	_, _ = configHash.Write(cfgBytes)

	return results
}

func buildConfig(params Params) ([]byte, error) {
	existingCfg, err := getExistingConfig(params.Context, params.Client, params.Logstash)
	if err != nil {
		return nil, err
	}

	userProvidedCfg, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	cfg, err := defaultConfig()
	if err != nil {
		return nil, err
	}

	// merge with user settings last so they take precedence
	if err = cfg.MergeWith(existingCfg, userProvidedCfg); err != nil {
		return nil, err
	}

	return cfg.Render()
}

// getExistingConfig retrieves the canonical config, if one exists
func getExistingConfig(ctx context.Context, client k8s.Client, logstash logstashv1alpha1.Logstash) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: logstash.Namespace,
		Name:      logstashv1alpha1.ConfigSecretName(logstash.Name),
	}
	err := client.Get(context.Background(), key, &secret)
	if err != nil && apierrors.IsNotFound(err) {
		ulog.FromContext(ctx).V(1).Info("Logstash config secret does not exist", "namespace", logstash.Namespace, "name", logstash.Name)
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	rawCfg, exists := secret.Data[LogstashConfigFileName]
	if !exists {
		return nil, nil
	}

	cfg, err := settings.ParseConfig(rawCfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Logstash.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Logstash.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Logstash, params.Logstash.Spec.ConfigRef, LogstashConfigFileName)
}

// TODO: remove testing value
func defaultConfig() (*settings.CanonicalConfig, error) {
	settingsMap := map[string]interface{}{
		"node.name": "test",
	}

	return settings.MustCanonicalConfig(settingsMap), nil
}