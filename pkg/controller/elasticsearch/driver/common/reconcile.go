// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package common contains shared reconciliation logic for Elasticsearch drivers.
package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/remotecluster"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/securitycontext"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

var (
	// DefaultRequeue is the default requeue result for reconciliation.
	DefaultRequeue = reconciler.ReconciliationState{Result: controller.Result{RequeueAfter: reconciler.DefaultRequeue}}
)

// ReconcileSharedResources contains the reconciliation logic shared by both stateful and stateless Elasticsearch drivers.
func ReconcileSharedResources(
	ctx context.Context,
	d commondriver.Interface,
	params driver.Parameters,
) (*ReconcileResult, *reconciler.Results) {
	results := reconciler.NewResult(ctx)
	log := ulog.FromContext(ctx)
	es := params.ES
	client := params.Client

	// Garbage collect secrets attached to this cluster that we don't need anymore.
	if err := cleanup.DeleteOrphanedSecrets(ctx, client, es); err != nil {
		return nil, results.WithError(err)
	}

	// Extract the metadata that should be propagated to children.
	meta := metadata.Propagate(&es, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&es))})

	// Reconcile the scripts ConfigMap.
	if err := configmap.ReconcileScriptsConfigMap(ctx, client, es, meta); err != nil {
		return nil, results.WithError(err)
	}

	// Reconcile transport service.
	if _, err := common.ReconcileService(ctx, client, services.NewTransportService(es, meta), &es); err != nil {
		return nil, results.WithError(err)
	}

	// Reconcile external service.
	externalService, err := common.ReconcileService(ctx, client, services.NewExternalService(es, meta), &es)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil, results.WithReconciliationState(DefaultRequeue.WithReason(fmt.Sprintf("Pending %s service recreation", services.ExternalServiceName(es.Name))))
		}
		return nil, results.WithError(err)
	}

	// Reconcile internal service.
	internalService, err := common.ReconcileService(ctx, client, services.NewInternalService(es, meta), &es)
	if err != nil {
		return nil, results.WithError(err)
	}

	// Remote Cluster Server (RCS2) Kubernetes Service reconciliation.
	if es.Spec.RemoteClusterServer.Enabled {
		// Remote Cluster Server is enabled, ensure that the related Kubernetes Service does exist.
		if _, err := common.ReconcileService(ctx, client, services.NewRemoteClusterService(es, meta), &es); err != nil {
			results.WithError(err)
		}
	} else {
		// Ensure that remote cluster Service does not exist.
		remoteClusterService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      services.RemoteClusterServiceName(es.Name),
			},
		}
		results.WithError(k8s.DeleteResourceIfExists(ctx, client, remoteClusterService))
	}

	// Get resources state
	resourcesState, err := reconcile.NewResourcesStateFromAPI(client, es)
	if err != nil {
		return nil, results.WithError(err)
	}

	WarnUnsupportedDistro(resourcesState.AllPods, params.ReconcileState.Recorder)

	// Reconcile users and roles
	controllerUser, err := user.ReconcileUsersAndRoles(
		ctx,
		client,
		es,
		params.DynamicWatches,
		params.Recorder,
		params.OperatorParameters.PasswordHasher,
		params.OperatorParameters.PasswordGenerator,
		meta)
	if err != nil {
		return nil, results.WithError(err)
	}

	// Reconcile HTTP certificates
	trustedHTTPCertificates, res := certificates.ReconcileHTTP(
		ctx,
		d,
		es,
		[]corev1.Service{*externalService, *internalService},
		params.OperatorParameters.GlobalCA,
		params.OperatorParameters.CACertRotation,
		params.OperatorParameters.CertRotation,
		meta,
	)
	results.WithResults(res)
	if res != nil && res.HasError() {
		return nil, results
	}

	// Start the ES observer
	minVersion, err := version.MinInPods(resourcesState.CurrentPods, label.VersionLabelName)
	if err != nil {
		return nil, results.WithError(err)
	}
	if minVersion == nil {
		minVersion = &params.Version
	}

	urlProvider := services.NewElasticsearchURLProvider(es, client)
	hasEndpoints := urlProvider.HasEndpoints()

	observedState := params.Observers.ObservedStateResolver(
		ctx,
		es,
		elasticsearchClientProvider(
			ctx,
			params,
			urlProvider,
			controllerUser,
			*minVersion,
			trustedHTTPCertificates,
		),
		hasEndpoints,
	)

	// Always update the Elasticsearch state bits with the latest observed state.
	params.ReconcileState.
		UpdateClusterHealth(observedState()). // Elasticsearch cluster health
		UpdateAvailableNodes(*resourcesState). // Available nodes
		UpdateMinRunningVersion(ctx, *resourcesState) // Min running version

	// Reconcile transport certificates
	res = certificates.ReconcileTransport(
		ctx,
		d,
		es,
		params.OperatorParameters.GlobalCA,
		params.OperatorParameters.CACertRotation,
		params.OperatorParameters.CertRotation,
		meta,
	)
	results.WithResults(res)
	if res != nil && res.HasError() {
		return nil, results
	}

	// Patch the Pods to add the expected node labels as annotations. Record the error, if any, but do not stop the
	// reconciliation loop as we don't want to prevent other updates from being applied to the cluster.
	results.WithResults(annotatePodsWithNodeLabels(ctx, client, es))

	// Verify the operator supports existing pods
	if err := VerifySupportsExistingPods(resourcesState.CurrentPods, params.SupportedVersions); err != nil {
		if !es.IsConfiguredToAllowDowngrades() {
			return nil, results.WithError(err)
		}
		log.Info("Allowing downgrade on user request", "warning", err.Error())
	}

	// Create ES client
	esClient := newElasticsearchClient(
		ctx,
		params,
		urlProvider,
		controllerUser,
		*minVersion,
		trustedHTTPCertificates,
	)

	// use unknown health as a proxy for a cluster not responding to requests
	hasKnownHealthState := observedState() != esv1.ElasticsearchUnknownHealth
	esReachable := hasEndpoints && hasKnownHealthState
	// report condition in Pod status
	if esReachable {
		params.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionTrue, esReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	} else {
		params.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, esReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	}

	// License check
	var currentLicense esclient.License
	if esReachable {
		currentLicense, err = license.CheckElasticsearchLicense(ctx, esClient)
		var e *license.GetLicenseError
		if errors.As(err, &e) {
			if !e.SupportedDistribution {
				msg := "Unsupported Elasticsearch distribution"
				// unsupported distribution, let's update the phase to "invalid" and stop the reconciliation
				params.ReconcileState.
					UpdateWithPhase(esv1.ElasticsearchResourceInvalid).
					AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
				esClient.Close()
				return nil, results.WithError(errors.Wrap(err, strings.ToLower(msg[0:1])+msg[1:]))
			}
			// update esReachable to bypass steps that requires ES up in order to not block reconciliation for long periods
			esReachable = e.EsReachable
		}
		if err != nil {
			msg := "Could not verify license, re-queuing"
			log.Info(msg, "err", err, "namespace", es.Namespace, "es_name", es.Name)
			params.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
			results.WithReconciliationState(DefaultRequeue.WithReason(msg))
		}
	}

	// Reconcile the Elasticsearch license (even if we assume the cluster might not respond to requests to cover the case of
	// expired licenses where all health API responses are 403)
	if hasEndpoints {
		err = license.Reconcile(ctx, client, es, esClient, currentLicense)
		if err != nil {
			msg := "Could not reconcile cluster license, re-queuing"
			// only log an event if Elasticsearch is in a state where success of this API call can be expected. The API call itself
			// will be logged by the client
			if hasKnownHealthState {
				log.Info(msg, "err", err, "namespace", es.Namespace, "es_name", es.Name)
				params.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
			}
			results.WithReconciliationState(DefaultRequeue.WithReason(msg))
		}
	}

	// Reconcile remote clusters
	if esReachable {
		requeue, err := remotecluster.UpdateSettings(ctx, client, esClient, params.Recorder, params.LicenseChecker, es)
		msg := "Could not update remote clusters in Elasticsearch settings, re-queuing"
		if err != nil {
			log.Info(msg, "err", err, "namespace", es.Namespace, "es_name", es.Name)
			params.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			results.WithError(err)
		}
		if requeue {
			results.WithReconciliationState(DefaultRequeue.WithReason("Updating remote cluster settings, re-queuing"))
		}
	}

	// Compute seed hosts based on current masters with a podIP
	if err := settings.UpdateSeedHostsConfigMap(ctx, client, es, resourcesState.AllPods, meta); err != nil {
		esClient.Close()
		return nil, results.WithError(err)
	}

	// File settings
	if params.Version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		requeue, err := maybeReconcileEmptyFileSettingsSecret(ctx, client, params.LicenseChecker, &es, params.OperatorParameters.OperatorNamespace)
		if err != nil {
			esClient.Close()
			return nil, results.WithError(err)
		} else if requeue {
			results.WithReconciliationState(
				DefaultRequeue.WithReason(
					fmt.Sprintf("This cluster is targeted by at least one StackConfigPolicy, expecting Secret %s to be created by StackConfigPolicy controller",
						esv1.FileSettingsSecretName(es.Name)),
				),
			)
		}
	}

	// Keystore
	keystoreParams := initcontainer.KeystoreParams
	keystoreSecurityContext := securitycontext.For(params.Version, true)
	keystoreParams.SecurityContext = &keystoreSecurityContext

	remoteClusterAPIKeys, err := apiKeyStoreSecretSource(ctx, &es, client)
	if err != nil {
		esClient.Close()
		return nil, results.WithError(err)
	}
	keystoreResources, err := keystore.ReconcileResources(
		ctx,
		d,
		&es,
		esv1.ESNamer,
		meta,
		keystoreParams,
		remoteClusterAPIKeys...,
	)
	if err != nil {
		esClient.Close()
		return nil, results.WithError(err)
	}

	// Cluster UUID
	requeue, err := bootstrap.ReconcileClusterUUID(ctx, client, &es, esClient, esReachable)
	if err != nil {
		esClient.Close()
		return nil, results.WithError(err)
	}
	if requeue {
		results = results.WithReconciliationState(DefaultRequeue.WithReason("Elasticsearch cluster UUID is not reconciled"))
	}

	// Stack monitoring
	err = stackmon.ReconcileConfigSecrets(ctx, client, es, meta)
	if err != nil {
		esClient.Close()
		return nil, results.WithError(err)
	}

	// Association check
	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, es.GetAssociations(), params.Recorder)
	if err != nil {
		esClient.Close()
		return nil, results.WithError(err)
	}
	if !areAssocsConfigured {
		results.WithReconciliationState(DefaultRequeue.WithReason("Some associations are not reconciled"))
	}

	return &ReconcileResult{
		Meta:              meta,
		ResourcesState:    resourcesState,
		ESClient:          esClient,
		ESReachable:       esReachable,
		KeystoreResources: keystoreResources,
	}, results
}

// maybeReconcileEmptyFileSettingsSecret reconciles an empty file-settings secret for this ES cluster
// based on license status and StackConfigPolicy targeting. When enterprise features are disabled always
// creates an empty file-settings secret. When enterprise features are enabled and at least one StackConfigPolicy
// targets this cluster returns true to requeue and doesn't create the empty file-settings secret. If no
// StackConfigPolicy targets this cluster it creates an empty file-settings secret. Note: This logic here prevents
// the race condition described in https://github.com/elastic/cloud-on-k8s/issues/8912.
func maybeReconcileEmptyFileSettingsSecret(ctx context.Context, c k8s.Client, licenseChecker commonlicense.Checker, es *esv1.Elasticsearch, operatorNamespace string) (bool, error) {
	// Check if file-settings secret already exists
	var currentSecret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: es.Namespace, Name: esv1.FileSettingsSecretName(es.Name)}, &currentSecret); err == nil {
		// Secret does exist
		return false, nil
	} else if !k8serrors.IsNotFound(err) {
		return false, err
	}

	log := ulog.FromContext(ctx)
	enabled, err := licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return false, err
	}
	if !enabled {
		// If the license is not enabled, we reconcile the empty file-settings secret
		return false, filesettings.ReconcileEmptyFileSettingsSecret(ctx, c, *es, true)
	}

	// Get all StackConfigPolicies in the cluster
	var policyList policyv1alpha1.StackConfigPolicyList
	if err := c.List(ctx, &policyList); err != nil {
		return false, err
	}

	// Check each policy to see if it targets this ES cluster
	for _, policy := range policyList.Items {
		// Check if this policy's namespace and label selector match this ES cluster
		matches, err := stackconfigpolicy.DoesPolicyMatchObject(&policy, es, operatorNamespace)
		if err != nil {
			// Do not return an err here as this potentially can block ES reconciliation if any SCP in the cluster
			// has an invalid label selector, even if it doesn't target the current elasticsearch cluster.
			log.Error(err, "Failed to check if StackConfigPolicy matches object", "scp_name", policy.Name, "scp_namespace", policy.Namespace)
			continue
		} else if !matches {
			continue
		}

		// Found a policy that targets this ES cluster but the file-settings secret does not exist.
		// Let the SCP controller manage it, however, return requeue true to handle the following edge case:
		// 1. SCP exists and targets ES cluster at creation time
		// 2. ES controller "defers" to SCP controller (doesn't create secret)
		// 3. SCP is deleted before it reconciles and creates the file-settings secret
		// 4. Result: ES cluster left without any file-settings secret
		return true, nil
	}

	// No policies target this cluster, so ES controller should create the empty secret
	return false, filesettings.ReconcileEmptyFileSettingsSecret(ctx, c, *es, true)
}

// apiKeyStoreSecretSource returns the Secret that holds the remote API keys.
func apiKeyStoreSecretSource(ctx context.Context, es *esv1.Elasticsearch, c k8s.Client) ([]commonv1.NamespacedSecretSource, error) {
	secretName := types.NamespacedName{
		Name:      esv1.RemoteAPIKeysSecretName(es.Name),
		Namespace: es.Namespace,
	}
	if err := c.Get(ctx, secretName, &corev1.Secret{}); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return []commonv1.NamespacedSecretSource{
		{
			Namespace:  es.Namespace,
			SecretName: secretName.Name,
		},
	}, nil
}

// esReachableConditionMessage returns a message describing the Elasticsearch reachability condition.
func esReachableConditionMessage(internalService *corev1.Service, isServiceReady bool, isRespondingToRequests bool) string {
	switch {
	case !isServiceReady:
		return fmt.Sprintf("Service %s/%s has no endpoint", internalService.Namespace, internalService.Name)
	case !isRespondingToRequests:
		return fmt.Sprintf("Service %s/%s has endpoints but Elasticsearch is unavailable", internalService.Namespace, internalService.Name)
	default:
		return fmt.Sprintf("Service %s/%s has endpoints", internalService.Namespace, internalService.Name)
	}
}
