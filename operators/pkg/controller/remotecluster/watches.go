// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"fmt"

	assoctype "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// addWatches set watched on objects needed to manage the association between a local and a remote cluster.
func addWatches(c controller.Controller, r *ReconcileClusterAssociation) error {
	// Watch for changes to RemoteCluster
	if err := c.Watch(&source.Kind{Type: &v1alpha1.RemoteCluster{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch TrustRelationships
	if err := c.Watch(&source.Kind{Type: &v1alpha1.TrustRelationship{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: newToRequestsFuncFromTrustRelationshipLabel(),
	}); err != nil {
		return err
	}

	// Watch Secrets objects in order to update the CA in the TrustRelationships.
	if err := c.Watch(&source.Kind{Type: &v1.Secret{}}, r.watches.Secrets); err != nil {
		return err
	}

	return nil
}

// newToRequestsFuncFromTrustRelationshipLabel creates a watch handler function that creates reconcile requests based on the
// labels set on the Trustrelationship object.
func newToRequestsFuncFromTrustRelationshipLabel() handler.ToRequestsFunc {
	return handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
		labels := obj.Meta.GetLabels()
		clusterAssociationName, ok := labels[RemoteClusterNameLabelName]
		if !ok {
			return nil
		}
		clusterAssociationNamespace, ok := labels[RemoteClusterNamespaceLabelName]
		if !ok {
			return nil
		}
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Namespace: clusterAssociationNamespace,
				Name:      clusterAssociationName},
			},
		}
	})
}

func watchName(clusterAssociation v1alpha1.RemoteCluster, elasticsearch assoctype.ObjectSelector) string {
	return fmt.Sprintf(
		"%s-%s-%s-%s",
		clusterAssociation.Namespace,
		clusterAssociation.Name,
		elasticsearch.Namespace,
		elasticsearch.Name,
	)
}

// addCertificatesAuthorityWatches sets some watches on all secrets containing the certificate of a CA involved in a association.
func addCertificatesAuthorityWatches(
	reconcileClusterAssociation *ReconcileClusterAssociation,
	clusterAssociation v1alpha1.RemoteCluster,
	cluster assoctype.ObjectSelector) error {
	// Watch the CA secret of Elasticsearch clusters which are involved in a association.
	err := reconcileClusterAssociation.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    watchName(clusterAssociation, cluster),
		Watched: getCASecretNamespacedName(cluster.NamespacedName()),
		Watcher: k8s.ExtractNamespacedName(&clusterAssociation),
	})
	if err != nil {
		return err
	}

	return nil
}
