// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"context"
	"fmt"
	"reflect"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	name = "remoteca-controller"

	EventReasonLocalCaCertNotFound = "LocalClusterCaNotFound"
	EventReasonRemoteCaCertMissing = "RemoteClusterCaNotFound"
)

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 20 * time.Second}
)

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *ReconcileRemoteCa {
	c := k8s.WrapClient(mgr.GetClient())
	return &ReconcileRemoteCa{
		Client:         c,
		accessReviewer: accessReviewer,
		scheme:         mgr.GetScheme(),
		watches:        watches.NewDynamicWatches(),
		recorder:       mgr.GetEventRecorderFor(name),
		licenseChecker: license.NewLicenseChecker(c, params.OperatorNamespace),
		Parameters:     params,
	}
}

var _ reconcile.Reconciler = &ReconcileRemoteCa{}

// ReconcileRemoteCa reconciles remote CA Secrets.
type ReconcileRemoteCa struct {
	k8s.Client
	operator.Parameters
	accessReviewer rbac.AccessReviewer
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	watches        watches.DynamicWatches
	licenseChecker license.Checker

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for the expected remote clusters in this Kubernetes cluster.
// It copies the remote CA Secrets so they can be trusted by every peer Elasticsearch clusters.
func (r *ReconcileRemoteCa) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "es_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "remoteca")
	defer tracing.EndTransaction(tx)

	// Fetch the local Elasticsearch spec
	es := esv1.Elasticsearch{}
	err := r.Get(request.NamespacedName, &es)
	if err != nil {
		if errors.IsNotFound(err) {
			return deleteAllRemoteCa(ctx, r, request.NamespacedName)
		}
		return reconcile.Result{}, err
	}

	if common.IsPaused(es.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name)
		return common.PauseRequeue, nil
	}

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled()
	if err != nil {
		return defaultRequeue, err
	}
	if !enabled {
		log.Info(
			"Remote cluster controller is an enterprise feature. Enterprise features are disabled",
			"namespace", es.Namespace, "es_name", es.Name,
		)
		return reconcile.Result{}, nil
	}

	return doReconcile(ctx, r, &es)
}

// deleteAllRemoteCa deletes all associated remote certificate authorities
func deleteAllRemoteCa(ctx context.Context, r *ReconcileRemoteCa, es types.NamespacedName) (reconcile.Result, error) {
	span, _ := apm.StartSpan(ctx, "delete_all_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	actualRemoteCertificateAuthorities, err := getActualRemoteCertificateAuthorities(ctx, r.Client, es)
	if err != nil {
		return reconcile.Result{}, err
	}
	results := &reconciler.Results{}
	for remoteCluster := range actualRemoteCertificateAuthorities {
		if err := deleteCertificateAuthorities(ctx, r, es, remoteCluster); err != nil {
			results.WithError(err)
		}
	}
	return results.Aggregate()
}

func doReconcile(
	ctx context.Context,
	r *ReconcileRemoteCa,
	localEs *esv1.Elasticsearch,
) (reconcile.Result, error) {
	localClusterKey := k8s.ExtractNamespacedName(localEs)
	// Get current clusters according to the existing remote CAs
	actualRemoteCertificateAuthorities, err := getActualRemoteCertificateAuthorities(ctx, r.Client, localClusterKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	expectedRemoteCertificateAuthorities, err := getExpectedRemoteCertificateAuthorities(ctx, r.Client, localEs)
	if err != nil {
		return reconcile.Result{}, err
	}

	results := &reconciler.Results{}
	// Create or update expected remote CA
	for remoteEsKey := range expectedRemoteCertificateAuthorities {
		// Get the remote Elasticsearch cluster associated with this remote CA
		remoteEs := &esv1.Elasticsearch{}
		if err := r.Client.Get(remoteEsKey, remoteEs); err != nil {
			if errors.IsNotFound(err) {
				// Remote cluster does not exist, skip it
				continue
			}
			return reconcile.Result{}, err
		}
		accessAllowed, err := isRemoteClusterAssociationAllowed(r.accessReviewer, localEs, remoteEs, r.recorder)
		if err != nil {
			return reconcile.Result{}, err
		}
		// if the remote CA exists but isn't allowed anymore, it will be deleted next
		if !accessAllowed {
			continue
		}
		delete(actualRemoteCertificateAuthorities, remoteEsKey)
		results.WithResults(createOrUpdateCertificateAuthorities(ctx, r, localEs, remoteEs))
		if results.HasError() {
			return results.Aggregate()
		}
	}

	// Delete existing but not expected remote CA
	for toDelete := range actualRemoteCertificateAuthorities {
		log.V(1).Info("Deleting remote CA",
			"local_namespace", localEs.Namespace,
			"local_name", localEs.Name,
			"remote_namespace", toDelete.Namespace,
			"remote_name", toDelete.Name,
		)
		results.WithError(deleteCertificateAuthorities(ctx, r, localClusterKey, toDelete))
	}
	return results.WithResult(association.RequeueRbacCheck(r.accessReviewer)).Aggregate()
}

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

// createOrUpdateCertificateAuthorities creates the Secrets that contains the remote certificate authorities
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
	if err := addCertificatesAuthorityWatches(r, remoteClusterKey, localClusterKey); err != nil {
		return results.WithError(err)
	}

	// Add watches on the CA secret of the remote cluster.
	if err := addCertificatesAuthorityWatches(r, localClusterKey, remoteClusterKey); err != nil {
		return results.WithError(err)
	}

	log.V(1).Info(
		"Setting up remote CA",
		"local_namespace", localClusterKey.Namespace,
		"local_name", localClusterKey.Namespace,
		"remote_namespace", remote.Namespace,
		"remote_name", remote.Name,
	)

	// Check if local CA exists
	localCA := &corev1.Secret{}
	if err := r.Client.Get(transport.PublicCertsSecretRef(localClusterKey), localCA); err != nil {
		if !errors.IsNotFound(err) {
			return results.WithError(err)
		}
		results.WithResult(defaultRequeue)
	}

	if len(localCA.Data[certificates.CAFileName]) == 0 {
		log.Info(
			"Cannot find local CA cert",
			"local_namespace", localClusterKey.Namespace,
			"local_name", localClusterKey.Namespace,
		)
		r.recorder.Event(local, v1.EventTypeWarning, EventReasonLocalCaCertNotFound, caCertMissingError("local", localClusterKey))
		// CA secrets are watched, we don't need to requeue.
		// If CA is created later it will trigger a new reconciliation.
		return nil
	}

	// Check if remote CA exists
	remoteCA := &corev1.Secret{}
	if err := r.Client.Get(transport.PublicCertsSecretRef(remoteClusterKey), remoteCA); err != nil {
		if !errors.IsNotFound(err) {
			return results.WithError(err)
		}
		results.WithResult(defaultRequeue)
	}

	if len(remoteCA.Data[certificates.CAFileName]) == 0 {
		log.Info("Cannot find remote CA cert",
			"remote_namespace", remote.Namespace,
			"remote_name", remote.Name,
		)
		r.recorder.Event(local, corev1.EventTypeWarning, EventReasonRemoteCaCertMissing, caCertMissingError("remote", remoteClusterKey))
		return nil
	}

	// Create local relationship
	if err := reconcileRemoteCA(ctx, r.Client, local, remoteClusterKey, remoteCA.Data[certificates.CAFileName]); err != nil {
		return results.WithError(err)
	}

	// Create remote relationship
	if err := reconcileRemoteCA(ctx, r.Client, remote, localClusterKey, localCA.Data[certificates.CAFileName]); err != nil {
		return results.WithError(err)
	}

	return nil
}

// reconcileRemoteCA copies the certificate authority from a remote cluster.
func reconcileRemoteCA(
	ctx context.Context,
	c k8s.Client,
	owner *esv1.Elasticsearch,
	remote types.NamespacedName,
	remoteCA []byte,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	// Define the expected remote CA object, it lives in the owner namespace with the content of the remote cluster CA
	expected := corev1.Secret{
		ObjectMeta: remoteCAObjectMeta(remoteCASecretName(owner.Name, remote), owner, remote),
		Data: map[string][]byte{
			certificates.CAFileName: remoteCA,
		},
	}

	var reconciled corev1.Secret
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme.Scheme,
		Owner:      owner,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !maps.IsSubset(expected.Labels, reconciled.Labels) || !reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Data = expected.Data
		},
	})
}

func caCertMissingError(location string, cluster types.NamespacedName) string {
	return fmt.Sprintf(
		"Cannot find CA certificate for %s cluster %s/%s",
		location,
		cluster.Namespace,
		cluster.Name,
	)
}

// getExpectedRemoteCertificateAuthorities returns all the remote cluster keys for which a remote ca should created
func getExpectedRemoteCertificateAuthorities(
	ctx context.Context,
	c k8s.Client,
	associatedEs *esv1.Elasticsearch,
) (map[types.NamespacedName]struct{}, error) {
	span, _ := apm.StartSpan(ctx, "get_expected_remote_ca", tracing.SpanTypeApp)
	defer span.End()
	expectedRemoteClusters := make(map[types.NamespacedName]struct{})

	// Add remote clusters declared in the Spec
	for _, remoteCluster := range associatedEs.Spec.RemoteClusters {
		if !remoteCluster.ElasticsearchRef.IsDefined() {
			continue
		}
		esRef := remoteCluster.ElasticsearchRef.WithDefaultNamespace(associatedEs.Namespace)
		expectedRemoteClusters[esRef.NamespacedName()] = struct{}{}
	}

	var list esv1.ElasticsearchList
	if err := c.List(&list, &client.ListOptions{}); err != nil {
		return nil, err
	}

	// Seek for Elasticsearch resources where this cluster is declared as a remote cluster
	for _, es := range list.Items {
		for _, remoteCluster := range es.Spec.RemoteClusters {
			if !remoteCluster.ElasticsearchRef.IsDefined() {
				continue
			}
			esRef := remoteCluster.ElasticsearchRef.WithDefaultNamespace(es.Namespace)
			if esRef.Namespace == associatedEs.Namespace &&
				esRef.Name == associatedEs.Name {
				expectedRemoteClusters[k8s.ExtractNamespacedName(&es)] = struct{}{}
			}
		}
	}

	return expectedRemoteClusters, nil
}

// getActualRemoteCertificateAuthorities returns all the Elasticsearch keys for which the remote certificate authorities have been copied.
// In order to get all of them we first list all the remote CA copied locally for a given, associated, Elasticsearch cluster.
// Then we list all the Elasticsearch clusters for which the CA of the associated cluster has been copied.
func getActualRemoteCertificateAuthorities(
	ctx context.Context,
	c k8s.Client,
	associatedEs types.NamespacedName,
) (map[types.NamespacedName]struct{}, error) {
	span, _ := apm.StartSpan(ctx, "get_current_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	currentRemoteClusters := make(map[types.NamespacedName]struct{})

	// 1. Get the remoteCA in the current namespace
	var remoteCAList corev1.SecretList
	if err := c.List(
		&remoteCAList,
		client.InNamespace(associatedEs.Namespace),
		LabelSelector(associatedEs.Name),
	); err != nil {
		return nil, err
	}
	for _, remoteCA := range remoteCAList.Items {
		remoteNs := remoteCA.Labels[RemoteClusterNamespaceLabelName]
		remoteEs := remoteCA.Labels[RemoteClusterNameLabelName]
		currentRemoteClusters[types.NamespacedName{
			Namespace: remoteNs,
			Name:      remoteEs,
		}] = struct{}{}
	}

	// 2. Get the remoteCA where this cluster is involved in other namespaces
	if err := c.List(
		&remoteCAList,
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:            TypeLabelValue,
			RemoteClusterNamespaceLabelName: associatedEs.Namespace,
			RemoteClusterNameLabelName:      associatedEs.Name,
		}),
	); err != nil {
		return nil, err
	}
	for _, remoteCA := range remoteCAList.Items {
		remoteEs := remoteCA.Labels[label.ClusterNameLabelName]
		currentRemoteClusters[types.NamespacedName{
			Namespace: remoteCA.Namespace,
			Name:      remoteEs,
		}] = struct{}{}
	}

	return currentRemoteClusters, nil
}
