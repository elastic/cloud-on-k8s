// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
)

// CertificateWatchKey returns the key used by the dynamic watch registration for custom http certificates
func CertificateWatchKey(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http-certificate")
}

// ReconcileCustomCertWatch takes a SecretRef and either creates or removes a dynamic watch for watchKey depending on
// whether secretRef empty or not.
func ReconcileCustomCertWatch(
	dynamicWatches watches.DynamicWatches,
	watchKey string,
	owner types.NamespacedName,
	tlsSecret commonv1.SecretRef,
) error {
	// watch the Secret specified in tlsSecret because if it changes we should reconcile the new
	// user provided certificates.
	httpCertificateWatch := watches.NamedWatch[*corev1.Secret]{
		Name: watchKey,
		Watched: []types.NamespacedName{{
			Namespace: owner.Namespace,
			Name:      tlsSecret.SecretName,
		}},
		Watcher: owner,
	}

	if tlsSecret.SecretName != "" {
		if err := dynamicWatches.Secrets.AddHandler(httpCertificateWatch); err != nil {
			return err
		}
	} else {
		// remove the watch if no longer configured.
		dynamicWatches.Secrets.RemoveHandlerForKey(httpCertificateWatch.Key())
	}

	return nil
}
