// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"k8s.io/apimachinery/pkg/types"
)

// CertificateWatchKey returns the key used by the dynamic watch registration for custom http certificates
func CertificateWatchKey(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http-certificate")
}

// reconcileDynamicWatches reconciles the dynamic watches needed by the HTTP certificates.
func reconcileDynamicWatches(dynamicWatches watches.DynamicWatches, owner types.NamespacedName, namer name.Namer, tls commonv1.TLSOptions) error {
	// watch the Secret specified in es.Spec.HTTP.TLS.Certificate because if it changes we should reconcile the new
	// user provided certificates.
	httpCertificateWatch := watches.NamedWatch{
		Name: CertificateWatchKey(namer, owner.Name),
		Watched: []types.NamespacedName{{
			Namespace: owner.Namespace,
			Name:      tls.Certificate.SecretName,
		}},
		Watcher: owner,
	}

	if tls.Certificate.SecretName != "" {
		if err := dynamicWatches.Secrets.AddHandler(httpCertificateWatch); err != nil {
			return err
		}
	} else {
		// remove the watch if no longer configured.
		dynamicWatches.Secrets.RemoveHandlerForKey(httpCertificateWatch.Key())
	}

	return nil
}
