// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"fmt"
	"regexp"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/types"
)

const (
	esWatchNameTemplate           = "%s-%s-es-watch-%s"
	esUserWatchNameTemplate       = "%s-%s-es-user-watch-%s"
	associatedCAWatchNameTemplate = "%s-%s-ca-watch-%s"
)

var (
	esWatchNameRegexp           = regexp.MustCompile(fmt.Sprintf(esWatchNameTemplate, "(.*)", "(.*)", `(\d+)`))
	esUserWatchNameRegexp       = regexp.MustCompile(fmt.Sprintf(esUserWatchNameTemplate, "(.*)", "(.*)", `(\d+)`))
	associatedCAWatchNameRegexp = regexp.MustCompile(fmt.Sprintf(associatedCAWatchNameTemplate, "(.*)", "(.*)", `(\d+)`))
)

// esWatchNameTemplate returns the name of the watch setup on the referenced Elasticsearch resource.
func esWatchName(associated types.NamespacedName, id string) string {
	return fmt.Sprintf(esWatchNameTemplate, associated.Namespace, associated.Name, id)
}

// esUserWatchNameTemplate returns the name of the watch setup on the ES user secret.
func esUserWatchName(associated types.NamespacedName, id string) string {
	return fmt.Sprintf(esUserWatchNameTemplate, associated.Namespace, associated.Name, id)
}

// associatedCAWatchNameTemplate returns the name of the watch setup on the secret of the associated resource that
// contains the HTTP certificate chain of Elasticsearch.
func associatedCAWatchName(associated types.NamespacedName, id string) string {
	return fmt.Sprintf(associatedCAWatchNameTemplate, associated.Namespace, associated.Name, id)
}

// setUserAndCaWatches sets up dynamic watches related to:
// * The referenced Elasticsearch resource
// * The user created in the Elasticsearch namespace
// * The CA of the target service (can be Kibana or Elasticsearch in the case of the APM)
func (r *Reconciler) setUserAndCaWatches(
	association commonv1.Association,
	esRef types.NamespacedName,
	remoteServiceNamer name.Namer,
) error {
	associatedKey := k8s.ExtractNamespacedName(association)

	id := association.ID()

	// watch the referenced ES cluster for future reconciliations
	if err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    esWatchName(associatedKey, id),
		Watched: []types.NamespacedName{esRef},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	// watch the user secret in the ES namespace
	userSecretKey := UserKey(association, esRef.Namespace, r.UserSecretSuffix)
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esUserWatchName(associatedKey, id),
		Watched: []types.NamespacedName{userSecretKey},
		Watcher: associatedKey,
	}); err != nil {
		return err
	}

	associationRef := association.AssociationRef()
	// watch the CA secret in the targeted service namespace
	// Most of the time it is Elasticsearch, but it could be Kibana in the case of the APMServer
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name: associatedCAWatchName(associatedKey, id),
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

func (r *Reconciler) removeWatchesExcept(associated types.NamespacedName, existing []commonv1.Association) {
	// - ES resource
	RemoveWatchesForDynamicRequest(associated, existing, esWatchNameRegexp, r.watches.ElasticsearchClusters)
	// - user in the ES namespace
	RemoveWatchesForDynamicRequest(associated, existing, esUserWatchNameRegexp, r.watches.Secrets)
	// - ES CA Secret in the ES namespace
	RemoveWatchesForDynamicRequest(associated, existing, associatedCAWatchNameRegexp, r.watches.Secrets)
}

// RemoveWatchesForDynamicRequest removes handlers in `dynamicRequest`. Handlers to remove are selected based
// on `regexp` and `associated`. Handlers related to any Association in `toKeep` won't be removed.
func RemoveWatchesForDynamicRequest(
	associated types.NamespacedName,
	toKeep []commonv1.Association,
	re *regexp.Regexp,
	dynamicRequest *watches.DynamicEnqueueRequest,
) {
	lookup := make(map[string]bool)
	for _, assoc := range toKeep {
		lookup[assoc.ID()] = true
	}

	for _, key := range dynamicRequest.Registrations() {
		matches := re.FindStringSubmatch(key)
		if len(matches) == 4 &&
			matches[1] == associated.Namespace &&
			matches[2] == associated.Name &&
			!lookup[matches[3]] {
			dynamicRequest.RemoveHandlerForKey(key)
		}
	}
}
