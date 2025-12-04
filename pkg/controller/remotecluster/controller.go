// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"fmt"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/autoops"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates/remoteca"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/remotecluster/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

const (
	name = "remotecluster-controller"

	EventReasonClusterCaCertNotFound = "ClusterCaCertNotFound"
)

// Add creates a new ReconcileRemoteClusters Controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := NewReconciler(mgr, accessReviewer, params)
	c, err := common.NewController(mgr, name, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *ReconcileRemoteClusters {
	c := mgr.GetClient()
	return &ReconcileRemoteClusters{
		Client:           c,
		accessReviewer:   accessReviewer,
		keystoreProvider: keystore.NewProvider(c),
		watches:          watches.NewDynamicWatches(),
		recorder:         mgr.GetEventRecorderFor(name),
		licenseChecker:   license.NewLicenseChecker(c, params.OperatorNamespace),
		Parameters:       params,
		esClientProvider: commonesclient.NewClient,
	}
}

var _ reconcile.Reconciler = &ReconcileRemoteClusters{}

// ReconcileRemoteClusters reconciles remote clusters Secrets and API Keys.
type ReconcileRemoteClusters struct {
	k8s.Client
	operator.Parameters
	accessReviewer   rbac.AccessReviewer
	recorder         record.EventRecorder
	watches          watches.DynamicWatches
	licenseChecker   license.Checker
	esClientProvider commonesclient.Provider
	keystoreProvider *keystore.Provider

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for the expected remote clusters in this Kubernetes cluster.
// It copies the remote CA Secrets so they can be trusted by every peer Elasticsearch clusters.
func (r *ReconcileRemoteClusters) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, name, "es_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// Fetch the local Elasticsearch spec
	es := esv1.Elasticsearch{}
	err := r.Get(ctx, request.NamespacedName, &es)
	if err != nil {
		if errors.IsNotFound(err) {
			r.keystoreProvider.ForgetCluster(request.NamespacedName)
			return deleteAllRemoteCa(ctx, r, request.NamespacedName)
		}
		return reconcile.Result{}, err
	}

	if common.IsUnmanaged(ctx, &es) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name)
		return reconcile.Result{}, nil
	}
	return doReconcile(ctx, r, &es)
}

// deleteAllRemoteCa deletes all associated remote certificate authorities
func deleteAllRemoteCa(ctx context.Context, r *ReconcileRemoteClusters, es types.NamespacedName) (reconcile.Result, error) {
	span, _ := apm.StartSpan(ctx, "delete_all_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	associatedCAs, err := getAssociatedRemoteCAs(ctx, r.Client, es)
	if err != nil {
		return reconcile.Result{}, err
	}
	results := &reconciler.Results{}
	for remoteCluster := range associatedCAs {
		if err := deleteCertificateAuthorities(ctx, r, es, remoteCluster); err != nil {
			results.WithError(err)
		}
	}
	return results.Aggregate()
}

func doReconcile(
	ctx context.Context,
	r *ReconcileRemoteClusters,
	remoteServer *esv1.Elasticsearch,
) (reconcile.Result, error) {
	log := ulog.FromContext(ctx)

	remoteServerKey := k8s.ExtractNamespacedName(remoteServer)

	expectedRemoteClients, err := getExpectedRemoteClientsFor(ctx, r.Client, remoteServer)
	if err != nil {
		return reconcile.Result{}, err
	}

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return reconcile.Result{RequeueAfter: reconciler.DefaultRequeue}, err
	}
	if !enabled && len(expectedRemoteClients) > 0 {
		log.V(1).Info(
			"Remote cluster controller is an enterprise feature. Enterprise features are disabled",
			"namespace", remoteServer.Namespace, "es_name", remoteServer.Name,
		)
		return reconcile.Result{}, nil
	}

	// Get all the clusters to which this reconciled cluster is connected to according to the existing remote CAs.
	// associatedRemoteCAs is used to delete the CA certificates and cancel any trust relationships
	// that may have existed in the past but should not exist anymore.
	associatedRemoteCAs, err := getAssociatedRemoteCAs(ctx, r.Client, remoteServerKey)
	if err != nil {
		return reconcile.Result{}, err
	}

	var (
		activeAPIKeys esclient.CrossClusterAPIKeyList
		esClient      esclient.Client
	)
	remoteServerSupportsClusterAPIKeys, err := remoteServer.SupportsRemoteClusterAPIKeys()
	if err != nil {
		return reconcile.Result{}, err
	}
	results := &reconciler.Results{}
	if remoteServerSupportsClusterAPIKeys.IsTrue() {
		// Check if the ES API is available. We need it to create, update and invalidate
		// API keys in this cluster.
		if !services.NewElasticsearchURLProvider(*remoteServer, r.Client).HasEndpoints() {
			log.Info("Elasticsearch API is not available yet")
			return results.WithRequeue().Aggregate()
		}
		// Create a new client
		newEsClient, err := r.esClientProvider(ctx, r.Client, r.Dialer, *remoteServer)
		if err != nil {
			return reconcile.Result{}, err
		}
		// Check that the API is available
		esClient = newEsClient
		// Get all the API Keys, for that specific client, on the reconciled cluster.
		crossClusterAPIKeys, err := esClient.GetCrossClusterAPIKeys(ctx, "eck-*")
		if err != nil {
			return reconcile.Result{}, err
		}
		activeAPIKeys = crossClusterAPIKeys
	}

	// apiKeyReconciledRemoteClients is used to track all the client clusters for which API keys have already been reconciled.
	// This is used to garbage collect API keys for clusters which have been deleted and are not in expectedRemoteClusters.
	apiKeyReconciledRemoteClients := sets.New[types.NamespacedName]()

	// Main loop to:
	// 1. Create or update expected remote CA.
	// 2. Create or update API keys and keystores.
	for remoteClientKey, remoteClusterRefs := range expectedRemoteClients {
		// Get the remote/client Elasticsearch cluster associated with this local/reconciled cluster.
		remoteClient := &esv1.Elasticsearch{}
		if err := r.Client.Get(ctx, remoteClientKey, remoteClient); err != nil {
			if errors.IsNotFound(err) {
				// Remote client cluster does not exist, invalidate API keys for that client cluster.
				apiKeyReconciledRemoteClients.Insert(remoteClientKey)
				results.WithResults(reconcileAPIKeys(ctx, r.Client, activeAPIKeys, remoteServer, remoteClient, nil, esClient, r.keystoreProvider))
				continue
			}
			return reconcile.Result{}, err
		}
		log := log.WithValues(
			"remote_server_namespace", remoteServer.Namespace,
			"remote_server", remoteServer.Name,
			"remote_client_namespace", remoteClient.Namespace,
			"remote_client_name", remoteClient.Name,
		)
		accessAllowed, err := isRemoteClusterAssociationAllowed(ctx, r.accessReviewer, remoteServer, remoteClient, r.recorder)
		if err != nil {
			return reconcile.Result{}, err
		}
		// if the remote CA exists but isn't allowed anymore, it will be deleted next
		if !accessAllowed {
			// Remove from the expected remote cluster to clean up local keystore.
			delete(expectedRemoteClients, remoteClientKey)
			// Invalidate API keys for that client cluster.
			apiKeyReconciledRemoteClients.Insert(remoteClientKey)
			results.WithResults(reconcileAPIKeys(ctx, r.Client, activeAPIKeys, remoteServer, remoteClient, nil, esClient, r.keystoreProvider))
			continue
		}
		delete(associatedRemoteCAs, remoteClientKey)
		results.WithResults(createOrUpdateCertificateAuthorities(ctx, r, remoteServer, remoteClient))
		if results.HasError() {
			return results.Aggregate()
		}

		// RCS2, first check that both the reconciled and the client clusters are compatible.
		clientClusterSupportsClusterAPIKeys, err := remoteClient.SupportsRemoteClusterAPIKeys()
		if err != nil {
			results.WithError(err)
			continue
		}

		if !clientClusterSupportsClusterAPIKeys.IsSet() {
			log.Info("Client cluster version is not available in status yet, skipping API keys reconciliation")
			continue
		}

		if !remoteServerSupportsClusterAPIKeys.IsSet() {
			log.Info("Cluster version is not available in status yet, skipping API keys reconciliation")
			continue
		}

		if clientClusterSupportsClusterAPIKeys.IsFalse() && remoteServerSupportsClusterAPIKeys.IsTrue() {
			err := fmt.Errorf("client cluster %s/%s is running version %s which does not support remote cluster keys", remoteClient.Namespace, remoteClient.Name, remoteClient.Spec.Version)
			log.Error(err, "cannot configure remote cluster settings")
			continue
		}
		// Reconcile the API Keys.
		apiKeyReconciledRemoteClients.Insert(remoteClientKey)
		results.WithResults(reconcileAPIKeys(ctx, r.Client, activeAPIKeys, remoteServer, remoteClient, remoteClusterRefs, esClient, r.keystoreProvider))
	}

	if remoteServerSupportsClusterAPIKeys.IsTrue() {
		// **************************************************************
		// Delete orphaned API keys from clusters which have been deleted
		// **************************************************************
		for _, activeAPIKey := range activeAPIKeys.APIKeys {
			// Skip API keys managed by the autoops controller.
			if activeAPIKey.Metadata != nil && activeAPIKey.Metadata[autoops.PolicyNameLabelKey] != nil {
				continue
			}
			clientCluster, err := activeAPIKey.GetElasticsearchName()
			if err != nil {
				results.WithError(err)
				continue
			}
			if _, exists := apiKeyReconciledRemoteClients[clientCluster]; exists {
				// API keys for that client cluster have already been reconciled, skip.
				continue
			}
			// This API key in the local cluster state belongs to an unknown cluster which is not expected and has not been reconciled.
			log.Info(fmt.Sprintf("Invalidating API key %s which belongs to unknown cluster %s", activeAPIKey.Name, clientCluster))
			results.WithError(esClient.InvalidateCrossClusterAPIKey(ctx, activeAPIKey.Name))
		}

		// *********************************************
		// Delete unexpected keys in the local keystore.
		// *********************************************
		expectedAliases := expectedAliases(remoteServer, expectedRemoteClients)
		apiKeyStore, err := r.keystoreProvider.ForCluster(ctx, log, remoteServer)
		if err != nil {
			return results.WithError(err).Aggregate()
		}

		for alias := range apiKeyStore.GetAliases() {
			if expectedAliases.Has(alias) {
				// Expected alias
				continue
			}
			// Unexpected
			log.Info(fmt.Sprintf("Removing unexpected remote API key %s", alias))
			apiKeyStore.Delete(alias)
		}
		results.WithResults(apiKeyStore.Save(ctx, r.Client, remoteServer))
	}

	// Delete existing but not expected remote CA
	for toDelete := range associatedRemoteCAs {
		log.V(1).Info("Deleting remote CA",
			"local_namespace", remoteServer.Namespace,
			"local_name", remoteServer.Name,
			"remote_namespace", toDelete.Namespace,
			"remote_name", toDelete.Name,
		)
		results.WithError(deleteCertificateAuthorities(ctx, r, remoteServerKey, toDelete))
	}
	return results.WithResult(association.RequeueRbacCheck(r.accessReviewer)).Aggregate()
}

func expectedAliases(
	localCluster *esv1.Elasticsearch,
	expectedRemoteCluster map[types.NamespacedName][]esv1.RemoteCluster,
) sets.Set[string] {
	aliases := sets.New[string]()
	for _, remoteCluster := range localCluster.Spec.RemoteClusters {
		clientClusterNamespacedName := remoteCluster.ElasticsearchRef.WithDefaultNamespace(localCluster.Namespace).NamespacedName()
		if _, ok := expectedRemoteCluster[clientClusterNamespacedName]; !ok {
			// Not expected, might have been filtered by RBAC rules
			continue
		}
		if remoteCluster.APIKey == nil {
			// Not using remote cluster server.
			continue
		}
		aliases.Insert(remoteCluster.Name)
	}
	return aliases
}

func caCertMissingError(cluster types.NamespacedName) string {
	return fmt.Sprintf("Cannot find CA certificate cluster %s/%s", cluster.Namespace, cluster.Name)
}

// getExpectedRemoteClientsFor returns all the remote cluster keys for which a remote ca and an API Key should be created.
// The CA certificates must be copied from the remote cluster to the local one and vice versa.
// The API Key is created in the remote cluster and injected in the keystore of the local cluster.
func getExpectedRemoteClientsFor(
	ctx context.Context,
	c k8s.Client,
	associatedEs *esv1.Elasticsearch,
) (map[types.NamespacedName][]esv1.RemoteCluster, error) {
	span, _ := apm.StartSpan(ctx, "get_expected_remote_clusters", tracing.SpanTypeApp)
	defer span.End()
	expectedRemoteClusters := make(map[types.NamespacedName][]esv1.RemoteCluster)

	// AddKey remote clusters declared in the Spec
	for _, remoteCluster := range associatedEs.Spec.RemoteClusters {
		if !remoteCluster.ElasticsearchRef.IsDefined() {
			continue
		}
		esRef := remoteCluster.ElasticsearchRef.WithDefaultNamespace(associatedEs.Namespace)
		expectedRemoteClusters[esRef.NamespacedName()] = nil
	}

	var list esv1.ElasticsearchList
	if err := c.List(ctx, &list, &client.ListOptions{}); err != nil {
		return nil, err
	}

	// Seek for Elasticsearch resources where this cluster is declared as a remote cluster
	for _, es := range list.Items {
		es := es
		for _, remoteCluster := range es.Spec.RemoteClusters {
			if !remoteCluster.ElasticsearchRef.IsDefined() {
				continue
			}
			esRef := remoteCluster.ElasticsearchRef.WithDefaultNamespace(es.Namespace)
			if esRef.Namespace == associatedEs.Namespace &&
				esRef.Name == associatedEs.Name {
				clientClusterName := k8s.ExtractNamespacedName(&es)
				expectedRemoteClusters[clientClusterName] = append(expectedRemoteClusters[clientClusterName], remoteCluster)
			}
		}
	}

	return expectedRemoteClusters, nil
}

// getAssociatedRemoteCAs returns for a given Elasticsearch cluster all the Elasticsearch keys for which
// the remote certificate authorities have been copied, i.e. all the other Elasticsearch clusters for which this cluster
// has been involved in a remote cluster association.
// In order to get all of them we:
// 1. List all the remote CA copied locally.
// 2. List all the other Elasticsearch clusters for which the CA of the given cluster has been copied.
func getAssociatedRemoteCAs(
	ctx context.Context,
	c k8s.Client,
	es types.NamespacedName,
) (map[types.NamespacedName]struct{}, error) {
	span, _ := apm.StartSpan(ctx, "get_current_remote_ca", tracing.SpanTypeApp)
	defer span.End()

	currentRemoteClusters := make(map[types.NamespacedName]struct{})

	// 1. Get clusters whose CA has been copied into the local namespace.
	var remoteCAList corev1.SecretList
	if err := c.List(ctx,
		&remoteCAList,
		client.InNamespace(es.Namespace),
		remoteca.Labels(es.Name),
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
	if err := c.List(ctx,
		&remoteCAList,
		client.MatchingLabels(map[string]string{
			commonv1.TypeLabelName:          remoteca.TypeLabelValue,
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
