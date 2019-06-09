// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"fmt"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// addWatches set watches on objects needed to manage the association between a local and a remote cluster.
func addWatches(c controller.Controller, r *ReconcileRemoteCluster) error {
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
	// Watch licenses in order to enable functionality if license status changes
	if err := r.watches.Secrets.AddHandler(license.NewWatch(allRemoteClustersMapper(r.Client))); err != nil {
		return err
	}

	// Watch Service resources in order to reconcile related remote clusters if it changes.
	if err := c.Watch(&source.Kind{Type: &v1.Service{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: allRemoteClustersWithMatchingSeedServiceMapper(r.Client),
	}); err != nil {
		return err
	}
	return nil
}

func allRemoteClustersWithMatchingSeedServiceMapper(c k8s.Client) handler.Mapper {
	return handler.ToRequestsFunc(func(object handler.MapObject) []reconcile.Request {
		// if it's a remote cluster seed service, we need to enqueue requests for any relevant remote clusters

		labels := object.Meta.GetLabels()
		if labels == nil {
			// not a remote cluster seed service, safe to ignore
			return nil
		}

		if _, ok := labels[RemoteClusterSeedServiceForLabelName]; !ok {
			// not a remote cluster seed service, safe to ignore
			return nil
		}

		svcNamespace := object.Meta.GetNamespace()
		svcName := object.Meta.GetName()

		var list v1alpha1.RemoteClusterList
		if err := c.List(&client.ListOptions{}, &list); err != nil {
			log.Error(err, "failed to list remote clusters in watch handler")
			// dropping any errors on the floor here
			return nil
		}

		var reqs []reconcile.Request
		for _, rc := range list.Items {
			// compare service name
			if remoteClusterSeedServiceName(rc.Spec.Remote.K8sLocalRef.Name) != svcName {
				continue
			}

			// compare service namespace, defaulting the remote service namespace to the remote cluster resource ns
			ns := rc.Spec.Remote.K8sLocalRef.Namespace
			if ns == "" {
				ns = rc.Namespace
			}
			if ns != svcNamespace {
				// service and remote namespace in different namespaces, so not relevant
				continue
			}

			log.Info("Synthesizing reconcile for ", "resource", k8s.ExtractNamespacedName(&rc))
			reqs = append(reqs, reconcile.Request{
				NamespacedName: k8s.ExtractNamespacedName(&rc),
			})
		}
		return reqs
	})
}

// allRemoteClustersMapper creates a reconcile request for each currently existing remote cluster resource.
func allRemoteClustersMapper(c k8s.Client) handler.Mapper {
	return handler.ToRequestsFunc(func(_ handler.MapObject) []reconcile.Request {
		var list v1alpha1.RemoteClusterList
		if err := c.List(&client.ListOptions{}, &list); err != nil {
			log.Error(err, "failed to list remote clusters in watch handler")
			// dropping any errors on the floor here
			return nil
		}
		var reqs []reconcile.Request
		for _, rc := range list.Items {
			log.Info("Synthesizing reconcile for ", "resource", k8s.ExtractNamespacedName(&rc))
			reqs = append(reqs, reconcile.Request{
				NamespacedName: k8s.ExtractNamespacedName(&rc),
			})
		}
		return reqs
	})
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

func watchName(clusterAssociation v1alpha1.RemoteCluster, elasticsearch commonv1alpha1.ObjectSelector) string {
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
	reconcileClusterAssociation *ReconcileRemoteCluster,
	clusterAssociation v1alpha1.RemoteCluster,
	cluster commonv1alpha1.ObjectSelector) error {
	// Watch the CA secret of Elasticsearch clusters which are involved in a association.
	err := reconcileClusterAssociation.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    watchName(clusterAssociation, cluster),
		Watched: transport.PublicCertsSecretRef(cluster.NamespacedName()),
		Watcher: k8s.ExtractNamespacedName(&clusterAssociation),
	})
	if err != nil {
		return err
	}

	return nil
}
