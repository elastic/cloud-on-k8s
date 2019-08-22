// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"k8s.io/apimachinery/pkg/types"
)

// httpCertificateWatchKey returns the key used by the dynamic watch registration for custom http certificates
func httpCertificateWatchKey(namer name.Namer, ownerName string) string {
	return namer.Suffix(ownerName, "http-certificate")
}

// reconcileDynamicWatches reconciles the dynamic watches needed by the HTTP certificates.
func reconcileDynamicWatches(dynamicWatches watches.DynamicWatches, owner types.NamespacedName, namer name.Namer, tls v1alpha1.TLSOptions) error {
	// watch the Secret specified in es.Spec.HTTP.TLS.Certificate because if it changes we should reconcile the new
	// user provided certificates.
	httpCertificateWatch := watches.NamedWatch{
		Name: httpCertificateWatchKey(namer, owner.Name),
		Watched: types.NamespacedName{
			Namespace: owner.Namespace,
			Name:      tls.Certificate.SecretName,
		},
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

// DynamicWatchesFinalizer returns a Finalizer for dynamic watches related to http certificates
func DynamicWatchesFinalizer(dynamicWatches watches.DynamicWatches, ownerName string, namer name.Namer) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "dynamic-watches.finalizers.k8s.elastic.co/http-certificates",
		Execute: func() error {
			// es resource is being finalized, so we no longer need the dynamic watch
			dynamicWatches.Secrets.RemoveHandlerForKey(httpCertificateWatchKey(namer, ownerName))
			return nil
		},
	}
}
