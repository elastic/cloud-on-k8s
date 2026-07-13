// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/remotecluster/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

// AddWatches set watches on objects needed to manage the association between a local and a remote cluster.
func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileRemoteClusters) error {
	m := r.NamespaceMatcher
	// Watch for changes to RemoteCluster
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &esv1.Elasticsearch{}, &handler.TypedEnqueueRequestForObject[*esv1.Elasticsearch]{})); err != nil {
		return err
	}

	// Emit changes to remote clusters to update API keys.
	if err := c.Watch(
		watches.NamespacedKind(
			m,
			mgr.GetCache(),
			&esv1.Elasticsearch{},
			handler.TypedEnqueueRequestsFromMapFunc[*esv1.Elasticsearch, reconcile.Request](
				func(ctx context.Context, elasticsearch *esv1.Elasticsearch) []reconcile.Request {
					requests := make([]reconcile.Request, 0, len(elasticsearch.Spec.RemoteClusters))
					for _, remoteCluster := range elasticsearch.Spec.RemoteClusters {
						requests = append(requests, reconcile.Request{NamespacedName: remoteCluster.ElasticsearchRef.WithDefaultNamespace(elasticsearch.Namespace).NamespacedName()})
					}
					return requests
				},
			),
		),
	); err != nil {
		return err
	}

	// Watch Secrets that contain:
	//  * Remote certificate authorities managed by this controller.
	//  * API keys
	if err := c.Watch(
		watches.NamespacedKind(m, mgr.GetCache(), &corev1.Secret{},
			handler.TypedEnqueueRequestsFromMapFunc[*corev1.Secret, reconcile.Request](newRequestsFromMatchedLabels()),
		)); err != nil {
		return err
	}

	// Dynamically watches the certificate authorities involved in a cluster relationship
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &corev1.Secret{}, r.watches.Secrets)); err != nil {
		return err
	}

	return r.watches.Secrets.AddHandlers(
		&watches.OwnerWatch[*corev1.Secret]{
			Scheme:       mgr.GetScheme(),
			Mapper:       mgr.GetRESTMapper(),
			OwnerType:    &esv1.Elasticsearch{},
			IsController: true,
		},
	)
}

// namespaceFlipRequests returns a mapper translating a namespace match-state change into
// reconcile requests for the Elasticsearch clusters affected by it: the ones living in the
// flipped namespace, the ones living elsewhere whose Spec.RemoteClusters reference a cluster
// in the flipped namespace, and the counterparts referenced by the flipped namespace's own
// clusters. Reconcile re-evaluates all of a cluster's remote cluster relationships in both
// directions (see getExpectedRemoteClientsFor), so enqueueing the counterparts is enough for
// them to establish or tear down trust; the FilterClient then decides what remains visible.
func namespaceFlipRequests(cache cache.Cache) func(context.Context, *corev1.Namespace) []reconcile.Request {
	return func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
		var list esv1.ElasticsearchList
		// List **cluster-wide** from the cache (not the FilterClient): affected clusters can
		// live in any matched namespace, and clusters in the namespace being de-scoped would
		// be hidden by the FilterClient.
		if err := cache.List(ctx, &list); err != nil {
			ulog.FromContext(ctx).Error(err, "Failed to list Elasticsearch clusters", "namespace", ns.Name)
			return nil
		}

		seen := make(map[types.NamespacedName]struct{})
		enqueue := func(nsn types.NamespacedName) {
			if _, ok := seen[nsn]; ok {
				return
			}
			seen[nsn] = struct{}{}
		}

		for _, es := range list.Items {
			if es.Namespace == ns.Name {
				// case A: the cluster lives in the flipped namespace.
				enqueue(k8s.ExtractNamespacedName(&es))

				// case B: the counterparts its own spec declares, wherever they live.
				for _, remoteCluster := range es.Spec.RemoteClusters {
					if !remoteCluster.ElasticsearchRef.IsSet() {
						continue
					}
					enqueue(remoteCluster.ElasticsearchRef.WithDefaultNamespace(es.Namespace).NamespacedName())
				}
				continue
			}

			// case C: the cluster lives elsewhere but references a cluster in the flipped
			// namespace. Its side of the relationship (remote CA copies, API keys) depends on
			// that counterpart's visibility, which just changed, so it must be re-reconciled.
			for _, remoteCluster := range es.Spec.RemoteClusters {
				if !remoteCluster.ElasticsearchRef.IsSet() {
					continue
				}
				// Compare the ref's effective namespace: WithDefaultNamespace resolves an
				// unset ref namespace the same way getExpectedRemoteClientsFor does, i.e. to
				// the declaring cluster's own namespace — which is not the flipped one here.
				if remoteCluster.ElasticsearchRef.WithDefaultNamespace(es.Namespace).NamespacedName().Namespace == ns.Name {
					enqueue(k8s.ExtractNamespacedName(&es))
					break
				}
			}
		}

		reqs := make([]reconcile.Request, 0, len(seen))
		for nsn := range seen {
			reqs = append(reqs, reconcile.Request{NamespacedName: nsn})
		}
		return reqs
	}
}

// newRequestsFromMatchedLabels creates a watch handler function that creates reconcile requests based on the
// labels set on a Secret which contains the remote CA.
func newRequestsFromMatchedLabels() handler.TypedMapFunc[*corev1.Secret, reconcile.Request] {
	return func(ctx context.Context, obj *corev1.Secret) []reconcile.Request {
		labels := obj.GetLabels()
		if maps.ContainsKeys(labels, RemoteClusterNameLabelName, RemoteClusterNamespaceLabelName, commonv1.TypeLabelName) {
			// Remote cluster CA
			if labels[commonv1.TypeLabelName] != remoteca.TypeLabelValue {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: labels[RemoteClusterNamespaceLabelName],
						Name:      labels[RemoteClusterNameLabelName],
					},
				},
			}
		}

		if maps.ContainsKeys(labels, label.ClusterNameLabelName, commonv1.TypeLabelName) {
			if labels[commonv1.TypeLabelName] != keystore.RemoteClusterAPIKeysType {
				return nil
			}
			// Remote cluster API keys Secret event.
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: obj.Namespace,
						Name:      labels[label.ClusterNameLabelName],
					},
				},
			}
		}

		return nil
	}
}

func watchName(local types.NamespacedName, remote types.NamespacedName) string {
	return fmt.Sprintf(
		"%s-%s-%s-%s",
		local.Namespace,
		local.Name,
		remote.Namespace,
		remote.Name,
	)
}

// addCertificatesAuthorityWatches sets some watches on all secrets containing the certificate of a CA involved in a association.
// The local CA is watched to update the trusted certificates in the remote clusters.
// The remote CAs are watched to update the trusted certificates of the local cluster.
func addCertificatesAuthorityWatches(
	reconcileClusterAssociation *ReconcileRemoteClusters,
	local, remote types.NamespacedName,
) error {
	// Watch the CA secret of Elasticsearch clusters which are involved in a association.
	err := reconcileClusterAssociation.watches.Secrets.AddHandler(watches.NamedWatch[*corev1.Secret]{
		Name:    watchName(local, remote),
		Watched: []types.NamespacedName{transport.PublicCertsSecretRef(remote)},
		Watcher: types.NamespacedName{
			Namespace: local.Namespace,
			Name:      local.Name,
		},
	})
	if err != nil {
		return err
	}

	return nil
}
