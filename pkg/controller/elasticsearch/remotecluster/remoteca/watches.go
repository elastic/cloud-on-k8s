// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// AddWatches set watches on objects needed to manage the association between a local and a remote cluster.
func AddWatches(c controller.Controller, r *ReconcileRemoteCa) error {
	// Watch for changes to RemoteCluster
	if err := c.Watch(&source.Kind{Type: &esv1.Elasticsearch{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch Secrets that contain remote certificate authorities managed by this controller
	if err := c.Watch(&source.Kind{Type: &v1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newToRequestsFuncFromSecret(),
	}); err != nil {
		return err
	}

	// Dynamically watches the certificate authorities involved in a cluster relationship
	if err := c.Watch(&source.Kind{Type: &v1.Secret{}}, r.watches.Secrets); err != nil {
		return err
	}
	if err := r.watches.Secrets.AddHandlers(
		&watches.OwnerWatch{
			EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &esv1.Elasticsearch{},
			},
		},
	); err != nil {
		return err
	}

	return nil
}

// newToRequestsFuncFromSecret creates a watch handler function that creates reconcile requests based on the
// labels set on a Secret which contains the remote CA.
func newToRequestsFuncFromSecret() handler.ToRequestsFunc {
	return func(obj handler.MapObject) []reconcile.Request {
		labels := obj.Meta.GetLabels()
		if !maps.ContainsKeys(labels, RemoteClusterNameLabelName, RemoteClusterNamespaceLabelName, common.TypeLabelName) {
			return nil
		}
		if labels[common.TypeLabelName] != TypeLabelValue {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Namespace: labels[RemoteClusterNamespaceLabelName],
				Name:      labels[RemoteClusterNameLabelName]},
			},
		}
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
	reconcileClusterAssociation *ReconcileRemoteCa,
	local, remote types.NamespacedName) error {
	// Watch the CA secret of Elasticsearch clusters which are involved in a association.
	err := reconcileClusterAssociation.watches.Secrets.AddHandler(watches.NamedWatch{
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
