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
	remoteclusterrbac "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/remotecluster/rbac"
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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	name = "remoteca-controller"

	EventReasonLocalCaCertNotFound = "LocalClusterCaNotFound"
	EventReasonRemoteCaCertMissing = "RemoteClusterCaNotFound"
	CaCertMissingError             = "Cannot find CA certificate for %s cluster %s/%s"
)

var (
	log            = logf.Log.WithName(name)
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 20 * time.Second}
)

// Add creates a new RemoteCluster Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := newReconciler(mgr, accessReviewer, params)
	c, err := add(mgr, r)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *ReconcileRemoteCa {
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

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return c, err
	}
	return c, nil
}

var _ reconcile.Reconciler = &ReconcileRemoteCa{}

// ReconcileRemoteCa reconciles a RemoteCluster object.
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

// Reconcile reads that state of the cluster for a RemoteCluster object and makes changes based on the state read
// and what is in the RemoteCluster.Spec
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

	// Use the driver to create the remote cluster
	return doReconcile(ctx, r, &es)
}

// deleteAllRemoteCa deletes all associated remote certificate authorities
func deleteAllRemoteCa(ctx context.Context, r *ReconcileRemoteCa, es types.NamespacedName) (reconcile.Result, error) {
	span, _ := apm.StartSpan(ctx, "delete_all_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	currentRemoteClusters, err := getCurrentRemoteCertificateAuthorities(ctx, r.Client, es)
	if err != nil {
		return reconcile.Result{}, err
	}
	results := &reconciler.Results{}
	for remoteCluster := range currentRemoteClusters {
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
	currentRemoteCertificateAuthorities, err := getCurrentRemoteCertificateAuthorities(ctx, r.Client, localClusterKey)
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
			if !errors.IsNotFound(err) {
				return reconcile.Result{}, err
			}
			// Remote cluster does not exist, skip it
			continue
		}
		accessAllowed, err := remoteclusterrbac.IsAssociationAllowed(r.accessReviewer, localEs, remoteEs, r.recorder)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !accessAllowed {
			continue
		}
		delete(currentRemoteCertificateAuthorities, remoteEsKey)
		results.WithResults(createOrUpdateCertificateAuthorities(ctx, r, localEs, remoteEs))
		if results.HasError() {
			return results.Aggregate()
		}
	}

	// Delete existing but not expected remote CA
	for toDelete := range currentRemoteCertificateAuthorities {
		log.V(1).Info("Delete remote CA",
			"localNamespace", localEs.Namespace,
			"localName", localEs.Name,
			"remoteNamespace", toDelete.Namespace,
			"remoteName", toDelete.Name,
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

// reconcileRemoteCA copies certificates authorities across 2 clusters
func reconcileRemoteCA(
	ctx context.Context,
	c k8s.Client,
	owner *esv1.Elasticsearch,
	remote types.NamespacedName,
	remoteCA []byte,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	// Define the desired remote CA object, it lives in the remote namespace.
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
		CaCertMissingError,
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
	for _, remoteCluster := range associatedEs.Spec.RemoteClusters.K8sLocal {
		if !remoteCluster.IsDefined() {
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
		for _, remoteCluster := range es.Spec.RemoteClusters.K8sLocal {
			if !remoteCluster.IsDefined() {
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

// getCurrentRemoteCertificateAuthorities returns all the remote cluster keys for which a remote ca exists
func getCurrentRemoteCertificateAuthorities(
	ctx context.Context,
	c k8s.Client,
	associatedEs types.NamespacedName,
) (map[types.NamespacedName]struct{}, error) {
	span, _ := apm.StartSpan(ctx, "get_current_remote_ca", tracing.SpanTypeApp)
	defer span.End()
	currentRemoteClusters := make(map[types.NamespacedName]struct{})

	// Get the remoteCA in the current namespace
	var remoteCAList corev1.SecretList
	if err := c.List(
		&remoteCAList,
		client.InNamespace(associatedEs.Namespace),
		GetRemoteCAMatchingLabel(associatedEs.Name),
	); err != nil {
		return nil, err
	}
	for _, remoteCA := range remoteCAList.Items {
		if remoteCA.Labels == nil {
			continue
		}
		remoteNs := remoteCA.Labels[RemoteClusterNamespaceLabelName]
		remoteEs := remoteCA.Labels[RemoteClusterNameLabelName]
		currentRemoteClusters[types.NamespacedName{
			Namespace: remoteNs,
			Name:      remoteEs,
		}] = struct{}{}
	}

	// Get the remoteCA where this cluster is involved in other namespaces
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
		if remoteCA.Labels == nil {
			continue
		}
		remoteEs := remoteCA.Labels[label.ClusterNameLabelName]
		currentRemoteClusters[types.NamespacedName{
			Namespace: remoteCA.Namespace,
			Name:      remoteEs,
		}] = struct{}{}
	}

	return currentRemoteClusters, nil
}
