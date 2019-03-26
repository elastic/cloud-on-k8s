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

func apply(
	rca *ReconcileRemoteCluster,
	remoteCluster v1alpha1.RemoteCluster,
) (v1alpha1.RemoteClusterStatus, error) {

	// Get the previous remote associated cluster, if the remote namespace has been updated by the user we must
	// delete the remote relationship from the old namespace and recreate it in the new namespace.
	if len(remoteCluster.Status.InClusterRemoteSelector.Namespace) > 0 &&
		remoteCluster.Spec.Remote.InRemoteCluster.Namespace != remoteCluster.Status.InClusterRemoteSelector.Namespace {
		log.V(1).Info("Remote cluster namespaced updated",
			"old", remoteCluster.Status.InClusterRemoteSelector.Namespace,
			"new", remoteCluster.Spec.Remote.InRemoteCluster.Namespace)
		previousRemoteRelationshipName := fmt.Sprintf(
			"%s-%s-%s",
			RemoteTrustRelationshipPrefix,
			remoteCluster.Name,
			remoteCluster.Namespace,
		)
		if err := ensureTrustRelationshipIsDeleted(
			rca.Client,
			previousRemoteRelationshipName,
			&remoteCluster,
			remoteCluster.Status.InClusterRemoteSelector); err != nil {
			return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterRemovalFailed), err
		}
	}

	var localClusterSelector assoctype.ObjectSelector
	// Get local cluster selector
	if localClusterName, ok := remoteCluster.Labels[label.ClusterNameLabelName]; !ok {
		log.Error(
			fmt.Errorf("missing local cluster label"),
			ClusterNameLabelMissing,
			"namespace", remoteCluster.Namespace,
			"name", remoteCluster.Name,
		)
		rca.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonConfigurationError, ClusterNameLabelMissing)
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), nil // Wait for the object to be updated
	} else {
		localClusterSelector = assoctype.ObjectSelector{
			Namespace: remoteCluster.Namespace,
			Name:      localClusterName,
		}
	}

	// Add finalizers used to remove watches and unset remote clusters settings.
	h := finalizer.NewHandler(rca)
	watchFinalizer := watchFinalizer(
		remoteCluster,
		localClusterSelector,
		remoteCluster.Spec.Remote.InRemoteCluster,
		rca.watches)
	err := h.Handle(&remoteCluster, watchFinalizer)
	if err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Add watches on the CA secret of the local cluster.
	if err := addCertificatesAuthorityWatches(rca, remoteCluster, localClusterSelector); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Add watches on the CA secret of the remote cluster.
	if err := addCertificatesAuthorityWatches(rca, remoteCluster, remoteCluster.Spec.Remote.InRemoteCluster); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	log.V(1).Info(
		"Remote cluster",
		"local_namespace", localClusterSelector.Namespace,
		"local_name", localClusterSelector.Namespace,
		"remote_namespace", remoteCluster.Spec.Remote.InRemoteCluster.Namespace,
		"remote_name", remoteCluster.Spec.Remote.InRemoteCluster.Name,
	)

	local, err := newAssociatedCluster(rca.Client, localClusterSelector)
	if err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	remote, err := newAssociatedCluster(rca.Client, remoteCluster.Spec.Remote.InRemoteCluster)
	if err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	if !remoteCluster.DeletionTimestamp.IsZero() {
		// association is being deleted nothing to do
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterDeletionPending), nil
	}

	// Check if local CA exists
	if local.CA == nil {
		message := fmt.Sprintf(
			CaCertMissingError,
			"local",
			localClusterSelector.Namespace,
			localClusterSelector.Name,
		)
		log.Error(fmt.Errorf("cannot find local Ca cert"), message)
		rca.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonLocalCaCertNotFound, message)
		// CA secrets are watched, we don't need to requeue.
		// If CA is created later it will trigger a new reconciliation.
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterPending), nil
	}

	// Check if remote CA exists
	if remote.CA == nil {
		message := fmt.Sprintf(
			CaCertMissingError, "remote",
			remoteCluster.Spec.Remote.InRemoteCluster.Namespace,
			remoteCluster.Spec.Remote.InRemoteCluster.Name,
		)
		log.Error(fmt.Errorf("cannot find remote Ca cert"), message)
		rca.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonRemoteCACertMissing, message)
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterPending), nil
	}

	// Create local relationship
	localSubject := nodecerts.GetSubjectName(remote.Selector.Name, remote.Selector.Namespace)
	localRelationshipName := fmt.Sprintf("%s-%s", LocalTrustRelationshipPrefix, remoteCluster.Name)
	if err := reconcileTrustRelationShip(rca.Client, &remoteCluster, localRelationshipName, local, remote, localSubject); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Create remote relationship
	remoteSubject := nodecerts.GetSubjectName(local.Selector.Name, local.Selector.Namespace)
	remoteRelationshipName := fmt.Sprintf("%s-%s-%s", RemoteTrustRelationshipPrefix, remoteCluster.Name, remoteCluster.Namespace)
	if err := reconcileTrustRelationShip(rca.Client, &remoteCluster, remoteRelationshipName, remote, local, remoteSubject); err != nil {
		return updateStatusWithState(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Build status
	status := v1alpha1.RemoteClusterStatus{
		State:                                v1alpha1.RemoteClusterPropagated,
		ClusterName:                          localClusterSelector.Name,
		LocalTrustRelationshipName:           localRelationshipName,
		InClusterRemoteSelector:              remote.Selector,
		InClusterRemoteTrustRelationshipName: remoteRelationshipName,
		SeedHosts:                            []string{services.ExternalDiscoveryServiceHostname(remote.Selector.NamespacedName())},
	}
	return status, nil
}

func updateStatusWithState(remoteCluster *v1alpha1.RemoteCluster, state string) v1alpha1.RemoteClusterStatus {
	status := remoteCluster.Status.DeepCopy()
	status.State = state
	return *status
}
