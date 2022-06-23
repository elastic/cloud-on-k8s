// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
)

// referencedResourceWatchName is the name of the watch set on the referenced resource.
func referencedResourceWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-referenced-resource-watch", associated.Namespace, associated.Name)
}

// referencedResourceWatchName is the name of the watch set on Secret containing the CA of the referenced resource.
func referencedResourceCASecretWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-referenced-resource-ca-secret-watch", associated.Namespace, associated.Name)
}

// esUserWatchName returns the name of the watch setup on the ES user secret.
func esUserWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-es-user-watch", associated.Namespace, associated.Name)
}

// serviceWatchName returns the name of the watch setup on the custom service to be used to make requests to the
// referenced resource.
func serviceWatchName(associated types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-svc-watch", associated.Namespace, associated.Name)
}

// reconcileWatches sets up dynamic watches for:
// * the referenced resource(s) managed or not by ECK (e.g. Elasticsearch for Kibana -> Elasticsearch associations)
// * the CA secret of the referenced resource in the referenced resource namespace
// * the referenced service to access the referenced resource
// * the referenced secret to access the referenced resource
// * if there's an ES user to create, watch the user Secret in ES namespace
// All watches for all given associations are set under the same watch name and replaced with each reconciliation.
// The given associations are expected to be of the same type (e.g. Kibana -> Elasticsearch, not Kibana -> Enterprise Search).
func (r *Reconciler) reconcileWatches(associated types.NamespacedName, associations []commonv1.Association) error {
	managedElasticRef := filterManagedElasticRef(associations)
	unmanagedElasticRef := filterUnmanagedElasticRef(associations)

	// we have 2 modes (exclusive) for the referenced resource: managed or not managed by ECK and referencedResourceWatchName is shared between both.
	// either watch the referenced resource managed by ECK
	if err := ReconcileWatch(associated, managedElasticRef, r.watches.ReferencedResources, referencedResourceWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		return association.AssociationRef().NamespacedName()
	}); err != nil {
		return err
	}
	// or watch the custom user secret that describes how to connect to the referenced resource not managed by ECK
	if err := ReconcileWatch(associated, unmanagedElasticRef, r.watches.Secrets, referencedResourceWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		return association.AssociationRef().NamespacedName()
	}); err != nil {
		return err
	}

	// watch the CA secret of the referenced resource in the referenced resource namespace
	if err := ReconcileWatch(associated, managedElasticRef, r.watches.Secrets, referencedResourceCASecretWatchName(associated), func(association commonv1.Association) types.NamespacedName {
		ref := association.AssociationRef()
		return types.NamespacedName{
			Name:      certificates.PublicCertsSecretName(r.AssociationInfo.ReferencedResourceNamer, ref.NameOrSecretName()),
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

	// watch the Elasticsearch user secret in the Elasticsearch namespace, if needed
	if r.ElasticsearchUserCreation != nil {
		if err := ReconcileWatch(associated, managedElasticRef, r.watches.Secrets, esUserWatchName(associated), func(association commonv1.Association) types.NamespacedName {
			return UserKey(association, association.AssociationRef().Namespace, r.ElasticsearchUserCreation.UserSecretSuffix)
		}); err != nil {
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
	// - referenced resource
	RemoveWatch(r.watches.ReferencedResources, referencedResourceWatchName(associated))
	// - CA secret in referenced resource namespace
	RemoveWatch(r.watches.Secrets, referencedResourceCASecretWatchName(associated))
	// - custom service watch in resource namespace
	RemoveWatch(r.watches.Services, serviceWatchName(associated))
	// - ES user secret
	RemoveWatch(r.watches.Secrets, esUserWatchName(associated))
}
