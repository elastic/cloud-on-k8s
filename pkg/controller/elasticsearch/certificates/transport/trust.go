// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// AdditionalCAWatchKey for config maps holding additional CAs for the transport layer.
func AdditionalCAWatchKey(name types.NamespacedName) string {
	return fmt.Sprintf("%s-transport-ca-trust", name)
}

func caWatchHandlerFor(name string, watched string, owner types.NamespacedName) watches.NamedWatch[*corev1.ConfigMap] {
	return watches.NamedWatch[*corev1.ConfigMap]{
		Name: name,
		Watched: []types.NamespacedName{{
			Namespace: owner.Namespace,
			Name:      watched,
		}},
		Watcher: owner,
	}
}

// ReconcileAdditionalCAs retrieves additional trust from an optional config map if configured and reconciles a watch for the config map.
func ReconcileAdditionalCAs(ctx context.Context, client k8s.Client, elasticsearch v1.Elasticsearch, watches watches.DynamicWatches) ([]byte, error) {
	esName := k8s.ExtractNamespacedName(&elasticsearch)
	watchKey := AdditionalCAWatchKey(esName)
	additionalTrust := elasticsearch.Spec.Transport.TLS.CertificateAuthorities
	if !additionalTrust.IsDefined() {
		watches.ConfigMaps.RemoveHandlerForKey(watchKey)
		return nil, nil
	}

	var configMap corev1.ConfigMap
	nsn := types.NamespacedName{Namespace: elasticsearch.Namespace, Name: additionalTrust.ConfigMapName}
	if err := client.Get(ctx, nsn, &configMap); err != nil {
		return nil, fmt.Errorf("could not retrieve config map %s specified in spec.transport.tls.certificateAuthorities: %w", nsn, err)
	}
	bytes, exists := configMap.Data[certificates.CAFileName]
	if !exists {
		return nil, fmt.Errorf("config map %s specified in spec.transport.tls.certificateAuthorities must contain ca.crt file", nsn)
	}
	return []byte(bytes), watches.ConfigMaps.AddHandler(caWatchHandlerFor(watchKey, additionalTrust.ConfigMapName, esName))
}
