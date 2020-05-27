// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/types"
)

// esWatchName returns the name of the watch setup on the referenced Elasticsearch resource.
func esWatchName(associated types.NamespacedName) string {
	return associated.Namespace + "-" + associated.Name + "-es-watch"
}

// esUserWatchName returns the name of the watch setup on the ES user secret.
func esUserWatchName(associated types.NamespacedName) string {
	return associated.Namespace + "-" + associated.Name + "-es-user-watch"
}

// associatedCAWatchName returns the name of the watch setup on the secret of the associated resource that
// contains the HTTP certificate chain of Elasticsearch.
func associatedCAWatchName(associated types.NamespacedName) string {
	return associated.Namespace + "-" + associated.Name + "-ca-watch"
}

// setUserAndCaWatches sets up dynamic watches related to:
// * The referenced Elasticsearch resource
// * The user created in the Elasticsearch namespace
// * The CA of the target service (can be Kibana or Elasticsearch in the case of the APM)
func (r *Reconciler) setUserAndCaWatches(
	association commonv1.Association,
	associationRef types.NamespacedName,
	esRef types.NamespacedName,
	remoteServiceNamer name.Namer,
) error {
	associatedKey := k8s.ExtractNamespacedName(association)

	// watch the referenced ES cluster for future reconciliations
	if err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    esWatchName(associatedKey),
		Watched: []types.NamespacedName{esRef},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	// watch the user secret in the ES namespace
	userSecretKey := UserKey(association, esRef.Namespace, r.UserSecretSuffix)
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esUserWatchName(associatedKey),
		Watched: []types.NamespacedName{userSecretKey},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	// watch the CA secret in the targeted service namespace
	// Most of the time it is Elasticsearch, but it could be Kibana in the case of the APMServer
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name: associatedCAWatchName(associatedKey),
		Watched: []types.NamespacedName{
			{
				Name:      certificates.PublicCertsSecretName(remoteServiceNamer, associationRef.Name),
				Namespace: associationRef.Namespace,
			},
		},
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
	r.watches.Secrets.RemoveHandlerForKey(associatedCAWatchName(associated))
	// - user in the ES namespace
	r.watches.Secrets.RemoveHandlerForKey(esUserWatchName(associated))
}
