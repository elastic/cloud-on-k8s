// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"context"
	"fmt"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	name = "remoteca-controller"

	EventReasonClusterCaCertNotFound = "ClusterCaCertNotFound"
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

	if common.IsUnmanaged(es.ObjectMeta) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name)
		return reconcile.Result{}, nil
	}

	return doReconcile(ctx, r, &es)
}

// deleteAllRemoteCa deletes all associated remote certificate authorities
func deleteAllRemoteCa(ctx context.Context, r *ReconcileRemoteCa, es types.NamespacedName) (reconcile.Result, error) {
	span, _ := apm.StartSpan(ctx, "delete_all_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	remoteClusters, err := remoteClustersInvolvedWith(ctx, r.Client, es)
	if err != nil {
		return reconcile.Result{}, err
	}
	results := &reconciler.Results{}
	for remoteCluster := range remoteClusters {
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

	expectedRemoteClusters, err := getExpectedRemoteClusters(ctx, r.Client, localEs)
	if err != nil {
		return reconcile.Result{}, err
	}

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled()
	if err != nil {
		return defaultRequeue, err
	}
	if !enabled && len(expectedRemoteClusters) > 0 {
		log.V(1).Info(
			"Remote cluster controller is an enterprise feature. Enterprise features are disabled",
			"namespace", localEs.Namespace, "es_name", localEs.Name,
		)
		return reconcile.Result{}, nil
	}

	// Get all the clusters to which this reconciled cluster is connected to according to the existing remote CAs.
	// remoteClustersInvolved is used to delete the CA certificates and cancel any trust relationships
	// that may have existed in the past but should not exist anymore.
	remoteClustersInvolved, err := remoteClustersInvolvedWith(ctx, r.Client, localClusterKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	results := &reconciler.Results{}
	// Create or update expected remote CA
	for remoteEsKey := range expectedRemoteClusters {
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
		delete(remoteClustersInvolved, remoteEsKey)
		results.WithResults(createOrUpdateCertificateAuthorities(ctx, r, localEs, remoteEs))
		if results.HasError() {
			return results.Aggregate()
		}
	}

	// Delete existing but not expected remote CA
	for toDelete := range remoteClustersInvolved {
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

func caCertMissingError(cluster types.NamespacedName) string {
	return fmt.Sprintf("Cannot find CA certificate cluster %s/%s", cluster.Namespace, cluster.Name)
}

// getExpectedRemoteClusters returns all the remote cluster keys for which a remote ca should created
// The CA certificates must be copied from the remote cluster to the local one and vice versa
func getExpectedRemoteClusters(
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

// remoteClustersInvolvedWith returns for a given Elasticsearch cluster all the Elasticsearch keys for which
// the remote certificate authorities have been copied, i.e. all the other Elasticsearch clusters for which this cluster
// has been involved in a remote cluster association.
// In order to get all of them we:
// 1. List all the remote CA copied locally.
// 2. List all the other Elasticsearch clusters for which the CA of the given cluster has been copied.
func remoteClustersInvolvedWith(
	ctx context.Context,
	c k8s.Client,
	es types.NamespacedName,
) (map[types.NamespacedName]struct{}, error) {
	span, _ := apm.StartSpan(ctx, "get_current_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	currentRemoteClusters := make(map[types.NamespacedName]struct{})

	// 1. Get clusters whose CA has been copied into the local namespace.
	var remoteCAList corev1.SecretList
	if err := c.List(
		&remoteCAList,
		client.InNamespace(es.Namespace),
		LabelSelector(es.Name),
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

	// 2. Get clusters for which the CA of the local cluster has been copied.
	if err := c.List(
		&remoteCAList,
		client.MatchingLabels(map[string]string{
			common.TypeLabelName:            TypeLabelValue,
			RemoteClusterNamespaceLabelName: es.Namespace,
			RemoteClusterNameLabelName:      es.Name,
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
