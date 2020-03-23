// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"context"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// createOrUpdateCertificateAuthorities creates the two Secrets that are needed to establish a trust relationship between
// two clusters. This is a bidirectional, symmetrical, action. In order to establish the trust relationship between
// a local and a remote cluster we must:
// * Copy the CA of the local cluster to the remote one.
// * Copy the CA of the remote cluster to the local one.
func createOrUpdateCertificateAuthorities(
	ctx context.Context,
	r *ReconcileRemoteCa,
	local, remote *esv1.Elasticsearch,
) *reconciler.Results {
	span, _ := apm.StartSpan(ctx, "create_or_update_remote_ca", tracing.SpanTypeApp)
	defer span.End()
	results := &reconciler.Results{}

	localClusterKey := k8s.ExtractNamespacedName(local)
	remoteClusterKey := k8s.ExtractNamespacedName(remote)

	// Add watches on the CA secret of the local cluster.
	if err := addCertificatesAuthorityWatches(r, localClusterKey, remoteClusterKey); err != nil {
		return results.WithError(err)
	}

	// Add watches on the CA secret of the remote cluster.
	if err := addCertificatesAuthorityWatches(r, remoteClusterKey, localClusterKey); err != nil {
		return results.WithError(err)
	}

	log.V(1).Info(
		"Setting up remote CA",
		"local_namespace", localClusterKey.Namespace,
		"local_name", localClusterKey.Namespace,
		"remote_namespace", remote.Namespace,
		"remote_name", remote.Name,
	)

	//  Copy CA from remote (source) to local (target) cluster
	if err := copyCertificateAuthority(ctx, r, remote, local); err != nil {
		if !errors.IsNotFound(err) {
			return results.WithError(err)
		}
		results.WithResult(defaultRequeue)
	}

	// Reciprocally, copy CA from local (source) to remote (target) cluster
	if err := copyCertificateAuthority(ctx, r, local, remote); err != nil {
		if !errors.IsNotFound(err) {
			return results.WithError(err)
		}
		results.WithResult(defaultRequeue)
	}

	return nil
}

// copyCertificateAuthority creates a copy of the CA from a source cluster to a target cluster
func copyCertificateAuthority(
	ctx context.Context,
	r *ReconcileRemoteCa,
	source, target *esv1.Elasticsearch,
) error {
	sourceKey := k8s.ExtractNamespacedName(source)
	// Check if CA of the source cluster exists
	sourceCA := &corev1.Secret{}
	if err := r.Client.Get(transport.PublicCertsSecretRef(sourceKey), sourceCA); err != nil {
		return err
	}

	if len(sourceCA.Data[certificates.CAFileName]) == 0 {
		log.Info(
			"Cannot find CA cert",
			"local_namespace", source.Namespace,
			"local_name", source.Namespace,
		)
		r.recorder.Event(source, corev1.EventTypeWarning, EventReasonClusterCaCertNotFound, caCertMissingError(sourceKey))
		// CA secrets are watched, we don't need to requeue.
		// If CA is created later it will trigger a new reconciliation.
		return nil
	}

	// Reconcile the copy to the target cluster
	if err := reconcileRemoteCA(ctx, r.Client, target, sourceKey, sourceCA.Data[certificates.CAFileName]); err != nil {
		return err
	}

	return nil
}

// deleteCertificateAuthorities deletes all the Secrets needed to establish a trust relationship between two clusters.
// This means that the CA of the local cluster is deleted from the remote one and reciprocally the CA from the
// remote cluster must be deleted from the local one.
func deleteCertificateAuthorities(
	ctx context.Context,
	r *ReconcileRemoteCa,
	local, remote types.NamespacedName,
) error {
	span, _ := apm.StartSpan(ctx, "delete_certificate_authorities", tracing.SpanTypeApp)
	defer span.End()

	// Delete local secret
	if err := r.Client.Delete(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: local.Namespace,
			Name:      remoteCASecretName(local.Name, remote),
		},
	}); err != nil && !errors.IsNotFound(err) {
		return err
	}
	// Delete remote secret
	if err := r.Client.Delete(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: remote.Namespace,
			Name:      remoteCASecretName(remote.Name, local),
		},
	}); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Remove watches
	r.watches.Secrets.RemoveHandlerForKey(watchName(local, remote))
	r.watches.Secrets.RemoveHandlerForKey(watchName(remote, local))

	return nil
}

// reconcileRemoteCA does the reconciliation of the Secret that contains certificate authority from a source cluster.
func reconcileRemoteCA(
	ctx context.Context,
	c k8s.Client,
	target *esv1.Elasticsearch,
	source types.NamespacedName,
	sourceCA []byte,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	// Define the expected source CA object, it lives in the target namespace with the content of the source cluster CA
	expected := corev1.Secret{
		ObjectMeta: remoteCAObjectMeta(remoteCASecretName(target.Name, source), target, source),
		Data: map[string][]byte{
			certificates.CAFileName: sourceCA,
		},
	}

	_, err := reconciler.ReconcileSecret(c, expected, target)
	return err
}
