// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateless

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func (sd *statelessDriver) getSecretSettings(ctx context.Context) (map[string]interface{}, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	watcher := k8s.ExtractNamespacedName(&sd.ES)
	// user-provided Secrets referenced in the resource
	secretSources := keystore.WatchedSecretNames(&sd.ES)
	if err := watches.WatchUserProvidedNamespacedSecrets(
		watcher,
		sd.DynamicWatches(),
		keystore.SecureSettingsWatchName(watcher),
		secretSources,
	); err != nil {
		return nil, err
	}

	secretSettings := make(map[string]interface{})
	for _, secureSetting := range secretSources {
		secureSettings := &corev1.Secret{}
		if err := sd.Client.Get(ctx, client.ObjectKey{
			Name:      secureSetting.SecretName,
			Namespace: sd.ES.Namespace,
		}, secureSettings); err != nil {
			return nil, err
		}
		for key, value := range secureSettings.Data {
			secretSettings[key] = string(value)
		}
	}
	return secretSettings, nil
}
