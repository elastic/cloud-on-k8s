// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	commondriver "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorejob"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/remotecluster"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/securitycontext"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/stackmon"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v3/pkg/dev"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/optional"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

var (
	defaultRequeue = reconciler.ReconciliationState{Result: controller.Result{RequeueAfter: reconciler.DefaultRequeue}}
)

// Driver orchestrates the reconciliation of an Elasticsearch resource.
// Its lifecycle is bound to a single reconciliation attempt.
type Driver interface {
	Reconcile(context.Context) *reconciler.Results
}

// NewDefaultDriver returns the default driver implementation.
func NewDefaultDriver(parameters DefaultDriverParameters) Driver {
	return &defaultDriver{DefaultDriverParameters: parameters}
}

// DefaultDriverParameters contain parameters for this driver.
// Most of them are persisted across driver creations.
type DefaultDriverParameters struct {
	// OperatorParameters contain global parameters about the operator.
	OperatorParameters operator.Parameters

	// ES is the Elasticsearch resource to reconcile
	ES esv1.Elasticsearch
	// SupportedVersions verifies whether we can support upgrading from the current pods.
	SupportedVersions version.MinMaxVersion

	// Version is the version of Elasticsearch we want to reconcile towards.
	Version version.Version
	// Client is used to access the Kubernetes API.
	Client   k8s.Client
	Recorder record.EventRecorder

	// LicenseChecker is used for some features to check if an appropriate license is setup
	LicenseChecker commonlicense.Checker

	// State holds the accumulated state during the reconcile loop
	ReconcileState *reconcile.State
	// Observers that observe es clusters state.
	Observers *observer.Manager
	// DynamicWatches are handles to currently registered dynamic watches.
	DynamicWatches watches.DynamicWatches
	// Expectations control some expectations set on resources in the cache, in order to
	// avoid doing certain operations if the cache hasn't seen an up-to-date resource yet.
	Expectations *expectations.Expectations
}

// defaultDriver is the default Driver implementation
type defaultDriver struct {
	DefaultDriverParameters
}

func (d *defaultDriver) K8sClient() k8s.Client {
	return d.Client
}

func (d *defaultDriver) DynamicWatches() watches.DynamicWatches {
	return d.DefaultDriverParameters.DynamicWatches
}

func (d *defaultDriver) Recorder() record.EventRecorder {
	return d.DefaultDriverParameters.Recorder
}

var _ commondriver.Interface = &defaultDriver{}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *defaultDriver) Reconcile(ctx context.Context) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	log := ulog.FromContext(ctx)

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(ctx, d.Client, d.ES); err != nil {
		return results.WithError(err)
	}

	// extract the metadata that should be propagated to children
	meta := metadata.Propagate(&d.ES, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&d.ES))})

	_, err := common.ReconcileService(ctx, d.Client, services.NewTransportService(d.ES, meta), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	externalService, err := common.ReconcileService(ctx, d.Client, services.NewExternalService(d.ES, meta), &d.ES)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return results.WithReconciliationState(defaultRequeue.WithReason(fmt.Sprintf("Pending %s service recreation", services.ExternalServiceName(d.ES.Name))))
		}
		return results.WithError(err)
	}

	var internalService *corev1.Service
	internalService, err = common.ReconcileService(ctx, d.Client, services.NewInternalService(d.ES, meta), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	// Remote Cluster Server (RCS2) Kubernetes Service reconciliation.
	if d.ES.Spec.RemoteClusterServer.Enabled {
		// Remote Cluster Server is enabled, ensure that the related Kubernetes Service does exist.
		if _, err := common.ReconcileService(ctx, d.Client, services.NewRemoteClusterService(d.ES, meta), &d.ES); err != nil {
			results.WithError(err)
		}
	} else {
		// Ensure that remote cluster Service does not exist.
		remoteClusterService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: d.ES.Namespace,
				Name:      services.RemoteClusterServiceName(d.ES.Name),
			},
		}
		results.WithError(k8s.DeleteResourceIfExists(ctx, d.Client, remoteClusterService))
	}

	resourcesState, err := reconcile.NewResourcesStateFromAPI(d.Client, d.ES)
	if err != nil {
		return results.WithError(err)
	}

	warnUnsupportedDistro(resourcesState.AllPods, d.ReconcileState.Recorder)

	controllerUser, err := user.ReconcileUsersAndRoles(
		ctx,
		d.Client,
		d.ES,
		d.DynamicWatches(),
		d.Recorder(),
		d.OperatorParameters.PasswordHasher,
		d.OperatorParameters.PasswordGenerator,
		meta)
	if err != nil {
		return results.WithError(err)
	}

	trustedHTTPCertificates, res := certificates.ReconcileHTTP(
		ctx,
		d,
		d.ES,
		[]corev1.Service{*externalService, *internalService},
		d.OperatorParameters.GlobalCA,
		d.OperatorParameters.CACertRotation,
		d.OperatorParameters.CertRotation,
		meta,
	)
	results.WithResults(res)
	if res != nil && res.HasError() {
		return results
	}

	// start the ES observer
	minVersion, err := version.MinInPods(resourcesState.CurrentPods, label.VersionLabelName)
	if err != nil {
		return results.WithError(err)
	}
	if minVersion == nil {
		minVersion = &d.Version
	}

	urlProvider := services.NewElasticsearchURLProvider(d.ES, d.Client)
	hasEndpoints := urlProvider.HasEndpoints()

	observedState := d.Observers.ObservedStateResolver(
		ctx,
		d.ES,
		d.elasticsearchClientProvider(
			ctx,
			urlProvider,
			controllerUser,
			*minVersion,
			trustedHTTPCertificates,
		),
		hasEndpoints,
	)

	// Always update the Elasticsearch state bits with the latest observed state.
	d.ReconcileState.
		UpdateClusterHealth(observedState()).         // Elasticsearch cluster health
		UpdateAvailableNodes(*resourcesState).        // Available nodes
		UpdateMinRunningVersion(ctx, *resourcesState) // Min running version

	res = certificates.ReconcileTransport(
		ctx,
		d,
		d.ES,
		d.OperatorParameters.GlobalCA,
		d.OperatorParameters.CACertRotation,
		d.OperatorParameters.CertRotation,
		meta,
	)
	results.WithResults(res)
	if res != nil && res.HasError() {
		return results
	}

	// Patch the Pods to add the expected node labels as annotations. Record the error, if any, but do not stop the
	// reconciliation loop as we don't want to prevent other updates from being applied to the cluster.
	results.WithResults(annotatePodsWithNodeLabels(ctx, d.Client, d.ES))

	if err := d.verifySupportsExistingPods(resourcesState.CurrentPods); err != nil {
		if !d.ES.IsConfiguredToAllowDowngrades() {
			return results.WithError(err)
		}
		log.Info("Allowing downgrade on user request", "warning", err.Error())
	}

	// TODO: support user-supplied certificate (non-ca)
	esClient := d.newElasticsearchClient(
		ctx,
		urlProvider,
		controllerUser,
		*minVersion,
		trustedHTTPCertificates,
	)
	defer esClient.Close()

	// use unknown health as a proxy for a cluster not responding to requests
	hasKnownHealthState := observedState() != esv1.ElasticsearchUnknownHealth
	esReachable := hasEndpoints && hasKnownHealthState
	// report condition in Pod status
	if esReachable {
		d.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionTrue, esReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	} else {
		d.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, esReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	}

	var currentLicense esclient.License
	if esReachable {
		currentLicense, err = license.CheckElasticsearchLicense(ctx, esClient)
		var e *license.GetLicenseError
		if errors.As(err, &e) {
			if !e.SupportedDistribution {
				msg := "Unsupported Elasticsearch distribution"
				// unsupported distribution, let's update the phase to "invalid" and stop the reconciliation
				d.ReconcileState.
					UpdateWithPhase(esv1.ElasticsearchResourceInvalid).
					AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
				return results.WithError(errors.Wrap(err, strings.ToLower(msg[0:1])+msg[1:]))
			}
			// update esReachable to bypass steps that requires ES up in order to not block reconciliation for long periods
			esReachable = e.EsReachable
		}
		if err != nil {
			msg := "Could not verify license, re-queuing"
			log.Info(msg, "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
			d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
			results.WithReconciliationState(defaultRequeue.WithReason(msg))
		}
	}

	// Update the service account orchestration hint. This is done early in the reconciliation loop to unblock association
	// controllers that may be waiting for the orchestration hint.
	results.WithError(d.maybeSetServiceAccountsOrchestrationHint(ctx, esReachable, esClient, resourcesState))

	// reconcile the Elasticsearch license (even if we assume the cluster might not respond to requests to cover the case of
	// expired licenses where all health API responses are 403)
	if hasEndpoints {
		err = license.Reconcile(ctx, d.Client, d.ES, esClient, currentLicense)
		if err != nil {
			msg := "Could not reconcile cluster license, re-queuing"
			// only log an event if Elasticsearch is in a state where success of this API call can be expected. The API call itself
			// will be logged by the client
			if hasKnownHealthState {
				log.Info(msg, "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
				d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
			}
			results.WithReconciliationState(defaultRequeue.WithReason(msg))
		}
	}

	// reconcile remote clusters
	if esReachable {
		requeue, err := remotecluster.UpdateSettings(ctx, d.Client, esClient, d.Recorder(), d.LicenseChecker, d.ES)
		msg := "Could not update remote clusters in Elasticsearch settings, re-queuing"
		if err != nil {
			log.Info(msg, "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
			d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			results.WithError(err)
		}
		if requeue {
			results.WithReconciliationState(defaultRequeue.WithReason("Updating remote cluster settings, re-queuing"))
		}
	}

	// Compute seed hosts based on current masters with a podIP
	if err := settings.UpdateSeedHostsConfigMap(ctx, d.Client, d.ES, resourcesState.AllPods, meta); err != nil {
		return results.WithError(err)
	}

	if d.Version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		requeue, err := maybeReconcileEmptyFileSettingsSecret(ctx, d.Client, d.LicenseChecker, &d.ES, d.OperatorParameters.OperatorNamespace)
		if err != nil {
			return results.WithError(err)
		} else if requeue {
			results.WithReconciliationState(
				defaultRequeue.WithReason(
					fmt.Sprintf("This cluster is targeted by at least one StackConfigPolicy, expecting Secret %s to be created by StackConfigPolicy controller",
						esv1.FileSettingsSecretName(d.ES.Name)),
				),
			)
		}
	}

	keystoreParams := initcontainer.KeystoreParams
	keystoreSecurityContext := securitycontext.For(d.Version, true)
	keystoreParams.SecurityContext = &keystoreSecurityContext

	// Set up a keystore with secure settings in an init container, if specified by the user.
	// We are also using the keystore internally for the remote cluster API keys.
	remoteClusterAPIKeys, err := apiKeyStoreSecretSource(ctx, &d.ES, d.Client)
	if err != nil {
		return results.WithError(err)
	}
	keystoreResources, err := keystore.ReconcileResources(
		ctx,
		d,
		&d.ES,
		esv1.ESNamer,
		meta,
		keystoreParams,
		remoteClusterAPIKeys...,
	)
	if err != nil {
		return results.WithError(err)
	}

	// For Elasticsearch 9.3+, use the job-based reloadable keystore approach instead of init containers.
	// This avoids pod restarts when secure settings change.
	keystoreConfig, keystoreSecretMountPath, err := d.reconcileReloadableKeystore(
		ctx, esReachable, esClient, keystoreResources, meta, results,
	)
	if err != nil {
		return results.WithError(err)
	}

	// Reconcile the scripts ConfigMap (must be after keystore decision to set up symlink correctly)
	if err := configmap.ReconcileScriptsConfigMap(ctx, d.Client, d.ES, meta, keystoreSecretMountPath); err != nil {
		return results.WithError(err)
	}

	// set an annotation with the ClusterUUID, if bootstrapped
	requeue, err := bootstrap.ReconcileClusterUUID(ctx, d.Client, &d.ES, esClient, esReachable)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results = results.WithReconciliationState(defaultRequeue.WithReason("Elasticsearch cluster UUID is not reconciled"))
	}

	// reconcile beats config secrets if Stack Monitoring is defined
	err = stackmon.ReconcileConfigSecrets(ctx, d.Client, d.ES, meta)
	if err != nil {
		return results.WithError(err)
	}

	// requeue if associations are defined but not yet configured, otherwise we may be in a situation where we deploy
	// Elasticsearch Pods once, then change their spec a few seconds later once the association is configured
	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, d.ES.GetAssociations(), d.Recorder())
	if err != nil {
		return results.WithError(err)
	}
	if !areAssocsConfigured {
		results.WithReconciliationState(defaultRequeue.WithReason("Some associations are not reconciled"))
	}

	// we want to reconcile suspended Pods before we start reconciling node specs as this is considered a debugging and
	// troubleshooting tool that does not follow the change budget restrictions
	if err := reconcileSuspendedPods(ctx, d.Client, d.ES, d.Expectations); err != nil {
		return results.WithError(err)
	}

	// reconcile StatefulSets and nodes configuration
	return results.WithResults(d.reconcileNodeSpecs(ctx, esReachable, esClient, d.ReconcileState, *resourcesState, keystoreConfig, meta))
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

// apiKeyStoreSecretSource returns the Secret that holds the remote API keys, and which should be used as a secure settings source.
func apiKeyStoreSecretSource(ctx context.Context, es *esv1.Elasticsearch, c k8s.Client) ([]commonv1.NamespacedSecretSource, error) {
	// Check if Secret exists
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

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(
	ctx context.Context,
	urlProvider esclient.URLProvider,
	user esclient.BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) esclient.Client {
	return esclient.NewElasticsearchClient(
		d.OperatorParameters.Dialer,
		k8s.ExtractNamespacedName(&d.ES),
		urlProvider,
		user,
		v,
		caCerts,
		esclient.Timeout(ctx, d.ES),
		dev.Enabled,
	)
}

func (d *defaultDriver) elasticsearchClientProvider(
	ctx context.Context,
	urlProvider esclient.URLProvider,
	user esclient.BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) func(existingEsClient esclient.Client) esclient.Client {
	return func(existingEsClient esclient.Client) esclient.Client {
		if existingEsClient != nil && existingEsClient.HasProperties(v, user, urlProvider, caCerts) {
			return existingEsClient
		}
		return d.newElasticsearchClient(ctx, urlProvider, user, v, caCerts)
	}
}

// maybeSetServiceAccountsOrchestrationHint attempts to update an orchestration hint to let the association controllers
// know whether all the nodes in the cluster are ready to authenticate service accounts.
func (d *defaultDriver) maybeSetServiceAccountsOrchestrationHint(
	ctx context.Context,
	esReachable bool,
	securityClient esclient.SecurityClient,
	resourcesState *reconcile.ResourcesState,
) error {
	if d.ReconcileState.OrchestrationHints().ServiceAccounts.IsTrue() {
		// Orchestration hint is already set to true, there is no point going back to false.
		return nil
	}

	// Case 1: New cluster, we can immediately set the orchestration hint.
	if !bootstrap.AnnotatedForBootstrap(d.ES) {
		allNodesRunningServiceAccounts, err := esv1.AreServiceAccountsSupported(d.ES.Spec.Version)
		if err != nil {
			return err
		}
		d.ReconcileState.UpdateOrchestrationHints(
			d.ReconcileState.OrchestrationHints().Merge(hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(allNodesRunningServiceAccounts)}),
		)
		return nil
	}

	// Case 2: This is an existing cluster, but actual cluster version does not support service accounts.
	if d.ES.Status.Version == "" {
		return nil
	}
	supportServiceAccounts, err := esv1.AreServiceAccountsSupported(d.ES.Status.Version)
	if err != nil {
		return err
	}
	if !supportServiceAccounts {
		d.ReconcileState.UpdateOrchestrationHints(
			d.ReconcileState.OrchestrationHints().Merge(hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(false)}),
		)
		return nil
	}

	// Case 3: cluster is already running with a version that does support service account and tokens have already been created.
	// We don't however know if all nodes have been migrated and are running with the service_tokens file mounted from the configuration Secret.
	// Let's try to detect that situation by comparing the existing nodes and the ones returned by the /_security/service API.
	// Note that starting with release 2.3 the association controller does not create the service account token until Elasticsearch is annotated
	// as compatible with service accounts. This is mostly to unblock situation described in https://github.com/elastic/cloud-on-k8s/issues/5684
	if !esReachable {
		// This requires the Elasticsearch API to be available
		return nil
	}
	allPods := names(resourcesState.AllPods)
	log := ulog.FromContext(ctx)
	// Detect if some service tokens are expected
	saTokens, err := user.GetServiceAccountTokens(d.Client, d.ES)
	if err != nil {
		log.Info("Could not detect if service accounts are expected", "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return err
	}

	allNodesRunningServiceAccounts, err := allNodesRunningServiceAccounts(ctx, saTokens, set.Make(allPods...), securityClient)
	if err != nil {
		log.Info("Could not detect if all nodes are ready for using service accounts", "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return err
	}
	if allNodesRunningServiceAccounts != nil {
		d.ReconcileState.UpdateOrchestrationHints(
			d.ReconcileState.OrchestrationHints().Merge(hints.OrchestrationsHints{ServiceAccounts: optional.NewBool(*allNodesRunningServiceAccounts)}),
		)
	}

	return nil
}

// allNodesRunningServiceAccounts attempts to detect if all the nodes in the clusters have loaded the service_tokens file.
// It returns nil if no decision can be made, for example when there is no tokens are expected to be found.
func allNodesRunningServiceAccounts(
	ctx context.Context,
	saTokens user.ServiceAccountTokens,
	allPods set.StringSet,
	securityClient esclient.SecurityClient,
) (*bool, error) {
	if len(allPods) == 0 {
		return nil, nil
	}
	if len(saTokens) == 0 {
		// No tokens are expected: we cannot call the Elasticsearch API to detect which nodes are
		// running with the conf/service_tokens file.
		return nil, nil
	}

	// Get the namespaced service name to call the /_security/service/<namespace>/<service>/credential API
	namespacedServices := saTokens.NamespacedServices()

	// Get the nodes which have loaded tokens from the conf/service_tokens file.
	for namespacedService := range namespacedServices {
		credentials, err := securityClient.GetServiceAccountCredentials(ctx, namespacedService)
		if err != nil {
			return nil, err
		}
		diff := allPods.Diff(credentials.Nodes())
		if len(diff) == 0 {
			return ptr.To[bool](true), nil
		}
	}
	// Some nodes are running but did not show up in the security API.
	return ptr.To[bool](false), nil
}

// warnUnsupportedDistro sends an event of type warning if the Elasticsearch Docker image is not a supported
// distribution by looking at if the prepare fs init container terminated with the UnsupportedDistro exit code.
func warnUnsupportedDistro(pods []corev1.Pod, recorder *events.Recorder) {
	for _, p := range pods {
		for _, s := range p.Status.InitContainerStatuses {
			state := s.LastTerminationState.Terminated
			if s.Name == initcontainer.PrepareFilesystemContainerName &&
				state != nil && state.ExitCode == initcontainer.UnsupportedDistroExitCode {
				recorder.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected,
					"Unsupported distribution")
			}
		}
	}
}

// reconcileReloadableKeystore handles the reloadable keystore feature for Elasticsearch 9.3+.
// It returns the keystore config, the mount path for the prepare-fs script, and any error.
func (d *defaultDriver) reconcileReloadableKeystore(
	ctx context.Context,
	esReachable bool,
	esClient esclient.Client,
	keystoreResources *keystore.Resources,
	meta metadata.Metadata,
	results *reconciler.Results,
) (nodespec.KeystoreConfig, string, error) {
	if !keystorejob.ShouldUseReloadableKeystore(ctx, d.ES, d.Version, keystoreResources, d.OperatorParameters.OperatorImage) {
		// Use the traditional init container approach
		return nodespec.KeystoreConfig{Resources: keystoreResources}, "", nil
	}

	// Reconcile the keystore job
	if _, err := d.reconcileKeystoreJob(ctx, keystoreResources, meta); err != nil {
		return nodespec.KeystoreConfig{}, "", err
	}

	// Don't block reconciliation while waiting for the keystore job - Kubernetes will
	// naturally keep new pods pending until the keystore Secret exists (volume mount dependency).
	// Existing pods continue running with their current keystore until the reload API is called.
	keystoreConfig := nodespec.KeystoreConfig{
		SecretName: esv1.KeystoreSecretName(d.ES.Name),
	}
	keystoreSecretMountPath := esvolume.KeystoreSecretVolumeMountPath

	// Trigger keystore reload on all ES nodes and check convergence
	if esReachable {
		expectedNodeCount := d.ES.Spec.NodeCount()
		reloadResult, err := keystorejob.ReloadSecureSettings(ctx, d.Client, esClient, d.ES, expectedNodeCount)
		if err != nil {
			return nodespec.KeystoreConfig{}, "", err
		}
		if !reloadResult.Converged {
			results.WithReconciliationState(defaultRequeue.WithReason("Keystore reload not yet converged: " + reloadResult.Message))
		}
	}

	return keystoreConfig, keystoreSecretMountPath, nil
}

// reconcileKeystoreJob reconciles the keystore creation Job for Elasticsearch 9.3+ clusters.
// It returns true if the keystore Secret is ready (job completed), false if still pending.
func (d *defaultDriver) reconcileKeystoreJob(ctx context.Context, keystoreResources *keystore.Resources, meta metadata.Metadata) (bool, error) {
	// Get the Elasticsearch image, preferring spec.image if set, otherwise using the default
	esImage := d.ES.Spec.Image
	if esImage == "" {
		esImage = container.ImageRepository(container.ElasticsearchImage, d.Version)
	}

	// Extract pod template params from the first node set for the Job
	var podTemplateParams keystorejob.JobPodTemplateParams
	if len(d.ES.Spec.NodeSets) > 0 {
		podSpec := d.ES.Spec.NodeSets[0].PodTemplate.Spec
		podTemplateParams = keystorejob.JobPodTemplateParams{
			ImagePullSecrets:   podSpec.ImagePullSecrets,
			PodSecurityContext: podSpec.SecurityContext,
		}
	}

	return keystorejob.ReconcileJob(ctx, keystorejob.Params{
		ES:                 d.ES,
		Client:             d.Client,
		OperatorNamespace:  d.OperatorParameters.OperatorNamespace,
		OperatorImage:      d.OperatorParameters.OperatorImage,
		ElasticsearchImage: esImage,
		KeystoreResources:  keystoreResources,
		Meta:               meta,
		PodTemplate:        podTemplateParams,
	})
}

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
