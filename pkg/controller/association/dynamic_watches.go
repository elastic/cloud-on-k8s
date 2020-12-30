// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"k8s.io/apimachinery/pkg/types"
)

// esWatchName returns the name of the watch setup on the referenced Elasticsearch resource.
func esWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-es-watch", associated.Namespace, associated.Name)
}

// esUserWatchName returns the name of the watch setup on the ES user secret.
func esUserWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-es-user-watch", associated.Namespace, associated.Name)
}

// associatedCAWatchName returns the name of the watch setup on the secret of the associated resource that
// contains the HTTP certificate chain of Elasticsearch.
func associatedCAWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-ca-watch", associated.Namespace, associated.Name)
}

// reconcileWatches sets up dynamic watches related to:
// * The referenced Elasticsearch resource
// * The user created in the Elasticsearch namespace
// * The CA of the target service (can be Kibana or Elasticsearch in the case of the APM)
// All watches for all Associations are set under the same watch name for associated resource and replaced
// with each reconciliation.
func (r *Reconciler) reconcileWatches(associated types.NamespacedName, associations []commonv1.Association) error {
	// watch the referenced ES cluster for future reconciliations
	if err := ReconcileWatch(associated, associations, r.watches.ElasticsearchClusters, esWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		return association.AssociationRef().NamespacedName()
	}); err != nil {
		return err
	}

	// watch the user secret in the ES namespace
	if err := ReconcileWatch(associated, associations, r.watches.Secrets, esUserWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		return UserKey(association, association.AssociationRef().Namespace, r.UserSecretSuffix)
	}); err != nil {
		return err
	}

	// watch the CA secret in the targeted service namespace
	// Most of the time it is Elasticsearch, but it could be Kibana in the case of the APMServer
	if err := ReconcileWatch(associated, associations, r.watches.Secrets, associatedCAWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		ref := association.AssociationRef()
		return types.NamespacedName{
			Name:      certificates.PublicCertsSecretName(r.AssociationInfo.AssociatedNamer, ref.Name),
			Namespace: ref.Namespace,
		}
	}); err != nil {
		return err
	}

	// set additional watches, in the case of a transitive Elasticsearch reference we must watch the intermediate resource
	if r.SetDynamicWatches != nil {
		if err := r.SetDynamicWatches(associated, associations, r.watches); err != nil {
			return err
		}
	}

	return nil
}

// ReconcileWatch sets or removes `watchName` watch in `dynamicRequest` based on `associated` and `associations` and
// `watchedFunc`.
func ReconcileWatch(
	associated types.NamespacedName,
	associations []commonv1.Association,
	dynamicRequest *watches.DynamicEnqueueRequest,
	watchName string,
	watchedFunc func(association commonv1.Association) types.NamespacedName,
) error {
	if len(associations) == 0 {
		// clean up if there are none
		RemoveWatch(dynamicRequest, watchName)
		return nil
	}

	toWatch := make([]types.NamespacedName, 0, len(associations))
	for _, association := range associations {
		toWatch = append(toWatch, watchedFunc(association))
	}

	return dynamicRequest.AddHandler(watches.NamedWatch{
		Name:    watchName,
		Watched: toWatch,
		Watcher: associated,
	})
}

// RemoveWatch removes `watchName` watch from `dynamicRequest`.
func RemoveWatch(dynamicRequest *watches.DynamicEnqueueRequest, watchName string) {
	dynamicRequest.RemoveHandlerForKey(watchName)
}

func (r *Reconciler) removeWatches(associated types.NamespacedName) {
	// - ES resource
	RemoveWatch(r.watches.ElasticsearchClusters, esWatchName(associated))
	// - user in the ES namespace
	RemoveWatch(r.watches.Secrets, esUserWatchName(associated))
	// - ES CA Secret in the ES namespace
	RemoveWatch(r.watches.Secrets, associatedCAWatchName(associated))
}
