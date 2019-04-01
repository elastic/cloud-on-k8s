// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"fmt"

	assoctype "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	v1 "k8s.io/api/core/v1"
)

const (
	LocalTrustRelationshipPrefix  = "rc"
	RemoteTrustRelationshipPrefix = "rcr"
)

func doReconcile(
	r *ReconcileRemoteCluster,
	remoteCluster v1alpha1.RemoteCluster,
) (v1alpha1.RemoteClusterStatus, error) {

	// Get the previous remote associated cluster, if the remote namespace has been updated by the user we must
	// delete the remote relationship from the old namespace and recreate it in the new namespace.
	if len(remoteCluster.Status.K8SLocalStatus.RemoteSelector.Namespace) > 0 &&
		remoteCluster.Spec.Remote.K8sLocalRef.Namespace != remoteCluster.Status.K8SLocalStatus.RemoteSelector.Namespace {
		log.V(1).Info("Remote cluster namespaced updated",
			"old", remoteCluster.Status.K8SLocalStatus.RemoteSelector.Namespace,
			"new", remoteCluster.Spec.Remote.K8sLocalRef.Namespace)
		previousRemoteRelationshipName := fmt.Sprintf(
			"%s-%s-%s",
			RemoteTrustRelationshipPrefix,
			remoteCluster.Name,
			remoteCluster.Namespace,
		)
		if err := ensureTrustRelationshipIsDeleted(
			r.Client,
			previousRemoteRelationshipName,
			remoteCluster,
			remoteCluster.Status.K8SLocalStatus.RemoteSelector); err != nil {
			return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterRemovalFailed), err
		}
	}

	var localClusterSelector assoctype.ObjectSelector
	// Get local cluster selector
	localClusterName, ok := remoteCluster.Labels[label.ClusterNameLabelName]
	if !ok {
		log.Error(
			fmt.Errorf("missing local cluster label"),
			ClusterNameLabelMissing,
			"namespace", remoteCluster.Namespace,
			"name", remoteCluster.Name,
		)
		r.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonConfigurationError, ClusterNameLabelMissing)
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), nil // Wait for the object to be updated
	}
	localClusterSelector = assoctype.ObjectSelector{
		Namespace: remoteCluster.Namespace,
		Name:      localClusterName,
	}

	// Add finalizers used to remove watches and unset remote clusters settings.
	h := finalizer.NewHandler(r)
	watchFinalizer := watchFinalizer(
		remoteCluster,
		localClusterSelector,
		remoteCluster.Spec.Remote.K8sLocalRef,
		r.watches)
	err := h.Handle(&remoteCluster, watchFinalizer)
	if err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Add watches on the CA secret of the local cluster.
	if err := addCertificatesAuthorityWatches(r, remoteCluster, localClusterSelector); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Add watches on the CA secret of the remote cluster.
	if err := addCertificatesAuthorityWatches(r, remoteCluster, remoteCluster.Spec.Remote.K8sLocalRef); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	log.V(1).Info(
		"Setting up remote cluster",
		"local_namespace", localClusterSelector.Namespace,
		"local_name", localClusterSelector.Namespace,
		"remote_namespace", remoteCluster.Spec.Remote.K8sLocalRef.Namespace,
		"remote_name", remoteCluster.Spec.Remote.K8sLocalRef.Name,
	)

	local, err := newAssociatedCluster(r.Client, localClusterSelector)
	if err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	remote, err := newAssociatedCluster(r.Client, remoteCluster.Spec.Remote.K8sLocalRef)
	if err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	if !remoteCluster.DeletionTimestamp.IsZero() {
		// association is being deleted nothing to do
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterDeletionPending), nil
	}

	// Check if local CA exists
	if local.CA == nil {
		message := caCertMissingError("local", localClusterSelector)
		log.Error(fmt.Errorf("cannot find local Ca cert"), message)
		r.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonLocalCaCertNotFound, message)
		// CA secrets are watched, we don't need to requeue.
		// If CA is created later it will trigger a new reconciliation.
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterPending), nil
	}

	// Check if remote CA exists
	if remote.CA == nil {
		message := caCertMissingError("remote", remoteCluster.Spec.Remote.K8sLocalRef)
		log.Error(fmt.Errorf("cannot find remote Ca cert"), message)
		r.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonRemoteCACertMissing, message)
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterPending), nil
	}

	// Create local relationship
	localSubject := nodecerts.GetSubjectName(remote.Selector.Name, remote.Selector.Namespace)
	localRelationshipName := fmt.Sprintf("%s-%s", LocalTrustRelationshipPrefix, remoteCluster.Name)
	if err := reconcileTrustRelationShip(r.Client, remoteCluster, localRelationshipName, local, remote, localSubject); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Create remote relationship
	remoteSubject := nodecerts.GetSubjectName(local.Selector.Name, local.Selector.Namespace)
	remoteRelationshipName := fmt.Sprintf("%s-%s-%s", RemoteTrustRelationshipPrefix, remoteCluster.Name, remoteCluster.Namespace)
	if err := reconcileTrustRelationShip(r.Client, remoteCluster, remoteRelationshipName, remote, local, remoteSubject); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Build status
	status := v1alpha1.RemoteClusterStatus{
		State:                  v1alpha1.RemoteClusterPropagated,
		ClusterName:            localClusterSelector.Name,
		LocalTrustRelationship: localRelationshipName,
		SeedHosts:              []string{services.ExternalDiscoveryServiceHostname(remote.Selector.NamespacedName())},
		K8SLocalStatus: v1alpha1.LocalRefStatus{
			RemoteSelector:          remote.Selector,
			RemoteTrustRelationship: remoteRelationshipName,
		},
	}
	return status, nil
}

func caCertMissingError(location string, selector assoctype.ObjectSelector) string {
	return fmt.Sprintf(
		CaCertMissingError,
		location,
		selector.Namespace,
		selector.Name,
	)
}

func updateStatusWithState(remoteCluster *v1alpha1.RemoteCluster, state string) v1alpha1.RemoteClusterStatus {
	status := remoteCluster.Status.DeepCopy()
	status.State = state
	return *status
}
