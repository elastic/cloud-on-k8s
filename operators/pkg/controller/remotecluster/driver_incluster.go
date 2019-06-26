// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"fmt"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	LocalTrustRelationshipPrefix  = "rc"
	RemoteTrustRelationshipPrefix = "rcr"

	// remoteClusterSeedServiceSuffix is the suffix used for the remote cluster seed service
	remoteClusterSeedServiceSuffix = "remote-cluster-seed"
)

func doReconcile(
	r *ReconcileRemoteCluster,
	remoteCluster v1alpha1.RemoteCluster,
) (v1alpha1.RemoteClusterStatus, error) {
	// Get the previous remote associated cluster, if the remote namespace has been updated by the user we must
	// delete the remote relationship from the old namespace and recreate it in the new namespace.
	if len(remoteCluster.Status.K8SLocalStatus.RemoteSelector.Namespace) > 0 &&
		remoteCluster.Spec.Remote.K8sLocalRef.Namespace != remoteCluster.Status.K8SLocalStatus.RemoteSelector.Namespace {
		log.V(1).Info("Remote cluster namespace updated",
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
			return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterRemovalFailed), err
		}
	}

	var localClusterSelector commonv1alpha1.ObjectSelector
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
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), nil // Wait for the object to be updated
	}
	localClusterSelector = commonv1alpha1.ObjectSelector{
		Namespace: remoteCluster.Namespace,
		Name:      localClusterName,
	}

	// Add finalizers used to remove watches and unset remote clusters settings.
	h := finalizer.NewHandler(r)
	watchFinalizer := watchFinalizer(
		remoteCluster,
		localClusterSelector,
		remoteCluster.Spec.Remote.K8sLocalRef,
		r.watches,
	)

	seedServiceFinalizer := seedServiceFinalizer(r.Client, remoteCluster)

	err := h.Handle(&remoteCluster, watchFinalizer, seedServiceFinalizer)
	if err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Add watches on the CA secret of the local cluster.
	if err := addCertificatesAuthorityWatches(r, remoteCluster, localClusterSelector); err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Add watches on the CA secret of the remote cluster.
	if err := addCertificatesAuthorityWatches(r, remoteCluster, remoteCluster.Spec.Remote.K8sLocalRef); err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
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
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	remote, err := newAssociatedCluster(r.Client, remoteCluster.Spec.Remote.K8sLocalRef)
	if err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	if !remoteCluster.DeletionTimestamp.IsZero() {
		// association is being deleted nothing to do
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterDeletionPending), nil
	}

	// Check if local CA exists
	if local.CA == nil {
		message := caCertMissingError("local", localClusterSelector)
		log.Error(fmt.Errorf("cannot find local Ca cert"), message)
		r.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonLocalCaCertNotFound, message)
		// CA secrets are watched, we don't need to requeue.
		// If CA is created later it will trigger a new reconciliation.
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterPending), nil
	}

	// Check if remote CA exists
	if remote.CA == nil {
		message := caCertMissingError("remote", remoteCluster.Spec.Remote.K8sLocalRef)
		log.Error(fmt.Errorf("cannot find remote Ca cert"), message)
		r.recorder.Event(&remoteCluster, v1.EventTypeWarning, EventReasonRemoteCACertMissing, message)
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterPending), nil
	}

	// Create local relationship
	localRelationshipName := fmt.Sprintf("%s-%s", LocalTrustRelationshipPrefix, remoteCluster.Name)
	if err := reconcileTrustRelationship(r.Client, remoteCluster, localRelationshipName, local, remote); err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Create remote relationship
	remoteRelationshipName := fmt.Sprintf("%s-%s-%s", RemoteTrustRelationshipPrefix, remoteCluster.Name, remoteCluster.Namespace)
	if err := reconcileTrustRelationship(r.Client, remoteCluster, remoteRelationshipName, remote, local); err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Create remote service for seeding
	svc, err := reconcileRemoteClusterSeedService(r.Client, r.scheme, remoteCluster)
	if err != nil {
		return updateStatusWithPhase(&remoteCluster, v1alpha1.RemoteClusterFailed), err
	}

	// Build status
	status := v1alpha1.RemoteClusterStatus{
		Phase:                  v1alpha1.RemoteClusterPropagated,
		ClusterName:            localClusterSelector.Name,
		LocalTrustRelationship: localRelationshipName,
		SeedHosts:              seedHostsFromService(svc),
		K8SLocalStatus: v1alpha1.LocalRefStatus{
			RemoteSelector:          remote.Selector,
			RemoteTrustRelationship: remoteRelationshipName,
		},
	}
	return status, nil
}

// reconcileRemoteClusterSeedService reconciles a Service that we can use as the remote cluster seed hosts.
//
// This service is shared between all remote clusters configured this way, and is deleted whenever any of them are
// deleted in a finalizer. There's a watch that re-creates it if it's still in use.
func reconcileRemoteClusterSeedService(
	c k8s.Client,
	scheme *runtime.Scheme,
	remoteCluster v1alpha1.RemoteCluster,
) (*v1.Service, error) {
	ns := remoteCluster.Spec.Remote.K8sLocalRef.Namespace
	// if the remote has no namespace, assume it's in the same namespace as the RemoteCluster resource
	if ns == "" {
		ns = remoteCluster.Namespace
	}
	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      remoteClusterSeedServiceName(remoteCluster.Spec.Remote.K8sLocalRef.Name),
			Labels: map[string]string{
				RemoteClusterSeedServiceForLabelName: remoteCluster.Spec.Remote.K8sLocalRef.Name,
			},
		},
		Spec: v1.ServiceSpec{
			PublishNotReadyAddresses: true,
			Ports: []v1.ServicePort{
				{Protocol: v1.ProtocolTCP, Port: 9300, TargetPort: intstr.FromInt(9300)},
			},
			Selector: map[string]string{
				common.TypeLabelName:       label.Type,
				label.ClusterNameLabelName: remoteCluster.Spec.Remote.K8sLocalRef.Name,
			},
		},
	}

	if _, err := common.ReconcileService(c, scheme, &service, nil); err != nil {
		return nil, err
	}

	return &service, nil
}

func caCertMissingError(location string, selector commonv1alpha1.ObjectSelector) string {
	return fmt.Sprintf(
		CaCertMissingError,
		location,
		selector.Namespace,
		selector.Name,
	)
}

func updateStatusWithPhase(
	remoteCluster *v1alpha1.RemoteCluster,
	phase v1alpha1.RemoteClusterPhase,
) v1alpha1.RemoteClusterStatus {
	status := remoteCluster.Status.DeepCopy()
	status.Phase = phase
	return *status
}

// remoteClusterSeedServiceName returns the name of the remote cluster seed service.
func remoteClusterSeedServiceName(esName string) string {
	return esname.ESNamer.Suffix(esName, remoteClusterSeedServiceSuffix)
}

// seedHostsFromService returns the seed hosts to use for a given service.
func seedHostsFromService(svc *v1.Service) []string {
	return []string{fmt.Sprintf("%s.%s.svc:9300", svc.Name, svc.Namespace)}
}
