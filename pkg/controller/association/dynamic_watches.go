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

// serviceWatchName returns the name of the watch monitor a custom service to be used to make requests to the
// associated resource.
func serviceWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-svc-watch", associated.Namespace, associated.Name)
}

// reconcileWatches sets up dynamic watches related to:
// * The referenced Elasticsearch resource
// * The user created in the Elasticsearch namespace
// * The CA of the target service (can be Kibana or Elasticsearch in the case of the APM)
// All watches for all Associations are set under the same watch name for associated resource and replaced
// with each reconciliation.
func (r *Reconciler) reconcileWatches(associated types.NamespacedName, associations []commonv1.Association) error {
	// watch the referenced ES cluster for future reconciliations
	// TODO: what if the referenced resource is not an Elasticsearch resource? https://github.com/elastic/cloud-on-k8s/issues/4591
	if err := ReconcileWatch(associated, associations, r.watches.ElasticsearchClusters, esWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		return association.AssociationRef().NamespacedName()
	}); err != nil {
		return err
	}

	if r.ElasticsearchUserCreation != nil {
		// watch the user secret in the ES namespace

		// TODO: this is not great. This reconcileWatches function is called by each association controller.
		// For example for Kibana, called both by the kb-es and kb-ent controllers.
		// Both controllers override each other's watches. It's OK when they write the same thing, but in this case
		// the kb-ent controller does not setup any es user while the kb-es controller does.
		// Because the context is different in both controller we end up with those weird if conditions.
		// Things would be more consistent if each association controller sets up its own watches rather than
		// having multiple association controllers setting up all associations watches for a given resource.
		// This way we'd just check r.ElasticsearchUserCreation != nil, rather than checking again later
		// for each association whether it requires auth.

		if err := ReconcileWatch(associated, associations, r.watches.Secrets, esUserWatchName(associated), func(association commonv1.Association) types.NamespacedName {
			if association.AssociationConf() == nil {
				// wait for an association to be configured first to figure out whether auth is required
				return types.NamespacedName{}
			}
			if association.AssociationConf().NoAuthRequired() {
				// this particular association does not require an es user
				return types.NamespacedName{}
			}
			return UserKey(association, association.AssociationRef().Namespace, r.ElasticsearchUserCreation.UserSecretSuffix)
		}); err != nil {
			return err
		}
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

	// watch the custom services users may have setup to be able to react to updates on services that are not error related
	// (error related updates are covered by re-queueing on unsuccessful reconciliation)
	if err := ReconcileWatch(associated, filterWithServiceName(associations), r.watches.Services, serviceWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		ref := association.AssociationRef()
		return types.NamespacedName{
			Name:      ref.ServiceName,
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
// `watchedFunc`. No watch is added if watchedFunc(association) refers to an empty namespaced name.
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

	emptyNamespacedName := types.NamespacedName{}

	toWatch := make([]types.NamespacedName, 0, len(associations))
	for _, association := range associations {
		watchedNamespacedName := watchedFunc(association)
		if watchedNamespacedName != emptyNamespacedName {
			toWatch = append(toWatch, watchedFunc(association))
		}
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
	// - custom service watch in resource namespace
	RemoveWatch(r.watches.Services, serviceWatchName(associated))
}
