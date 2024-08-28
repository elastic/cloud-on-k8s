// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remoteca

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/certificates/remoteca"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// AddWatches set watches on objects needed to manage the association between a local and a remote cluster.
func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileRemoteCa) error {
	// Watch for changes to RemoteCluster
	if err := c.Watch(source.Kind(mgr.GetCache(), &esv1.Elasticsearch{}, &handler.TypedEnqueueRequestForObject[*esv1.Elasticsearch]{})); err != nil {
		return err
	}

	// Watch Secrets that contain remote certificate authorities managed by this controller
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &v1.Secret{},
			handler.TypedEnqueueRequestsFromMapFunc[*v1.Secret, reconcile.Request](newRequestsFromMatchedLabels()),
		)); err != nil {
		return err
	}

	// Dynamically watches the certificate authorities involved in a cluster relationship
	if err := c.Watch(source.Kind(mgr.GetCache(), &v1.Secret{}, r.watches.Secrets)); err != nil {
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

// newRequestsFromMatchedLabels creates a watch handler function that creates reconcile requests based on the
// labels set on a Secret which contains the remote CA.
func newRequestsFromMatchedLabels() handler.TypedMapFunc[*v1.Secret, reconcile.Request] {
	return handler.TypedMapFunc[*v1.Secret, reconcile.Request](func(ctx context.Context, obj *v1.Secret) []reconcile.Request {
		labels := obj.GetLabels()
		if !maps.ContainsKeys(labels, RemoteClusterNameLabelName, RemoteClusterNamespaceLabelName, commonv1.TypeLabelName) {
			return nil
		}
		if labels[commonv1.TypeLabelName] != remoteca.TypeLabelValue {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Namespace: labels[RemoteClusterNamespaceLabelName],
				Name:      labels[RemoteClusterNameLabelName]},
			},
		}
	})
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
	reconcileClusterAssociation *ReconcileRemoteCa,
	local, remote types.NamespacedName) error {
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
