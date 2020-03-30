// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// esWatchName returns the name of the watch setup on the referenced Elasticsearch resource.
func esWatchName(associated types.NamespacedName) string {
	return associated.Namespace + "-" + associated.Name + "-es-watch"
}

// esUserWatchName returns the name of the watch setup on the ES user secret.
func esUserWatchName(associated types.NamespacedName) string {
	return associated.Namespace + "-" + associated.Name + "-es-watch"
}

// esCAWatchName returns the name of the watch setup on the secret that
// contains the HTTP certificate chain of Elasticsearch.
func esCAWatchName(associated types.NamespacedName) string {
	return associated.Namespace + "-" + associated.Name + "-ca-watch"
}

// setDynamicWatches sets up dynamic watches related to a referenced Elasticsearch resource.
func (r *Reconciler) setDynamicWatches(associated commonv1.Associated, esRef types.NamespacedName) error {
	associatedKey := k8s.ExtractNamespacedName(associated)

	// watch the referenced ES cluster for future reconciliations
	if err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    esWatchName(associatedKey),
		Watched: []types.NamespacedName{esRef},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	// watch the user secret in the ES namespace
	userSecretKey := UserKey(associated, r.UserSecretSuffix)
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esUserWatchName(associatedKey),
		Watched: []types.NamespacedName{userSecretKey},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	// watch ES CA secret in the ES namespace
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(associatedKey),
		Watched: []types.NamespacedName{certificates.PublicCertsSecretRef(esv1.ESNamer, esRef)},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) removeWatches(associated types.NamespacedName) {
	// - ES resource
	r.watches.ElasticsearchClusters.RemoveHandlerForKey(esWatchName(associated))
	// - ES CA Secret in the ES namespace
	r.watches.Secrets.RemoveHandlerForKey(esCAWatchName(associated))
	// - user in the ES namespace
	r.watches.Secrets.RemoveHandlerForKey(esUserWatchName(associated))
}
