// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/types"
)

// httpCertificateWatchKey returns the key used by the dynamic watch registration for custom http certificates
func httpCertificateWatchKey(es v1alpha1.Elasticsearch) string {
	return fmt.Sprintf("%s-%s-http-certificate", es.Namespace, es.Name)
}

// handleDynamicWatches reconciles the dynamic watches needed by the resource.
func (r *ReconcileElasticsearch) handleDynamicWatches(es v1alpha1.Elasticsearch) error {
	// watch the Secret specified in es.Spec.HTTP.TLS.Certificate because if it changes we should reconcile the new
	// user provided certificates.
	httpCertificateWatch := watches.NamedWatch{
		Name: httpCertificateWatchKey(es),
		Watched: types.NamespacedName{
			Namespace: es.Namespace,
			Name:      es.Spec.HTTP.TLS.Certificate.SecretName,
		},
		Watcher: k8s.ExtractNamespacedName(&es),
	}

	if es.Spec.HTTP.TLS.Certificate.SecretName != "" {
		if err := r.dynamicWatches.Secrets.AddHandler(httpCertificateWatch); err != nil {
			return err
		}
	} else {
		// remove the watch if no longer configured.
		r.dynamicWatches.Secrets.RemoveHandlerForKey(httpCertificateWatch.Key())
	}

	return nil
}

// dynamicWatchesFinalizerFor returns a Finalizer for dynamic watches related to a specific resource
func (r *ReconcileElasticsearch) dynamicWatchesFinalizerFor(es v1alpha1.Elasticsearch) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "dynamic-watches.finalizers.elasticsearch.k8s.elastic.co",
		Execute: func() error {
			// es resource is being finalized, so we no longer need the dynamic watch
			r.dynamicWatches.Secrets.RemoveHandlerForKey(httpCertificateWatchKey(es))
			return nil
		},
	}
}
