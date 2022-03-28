// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commondriver "github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	commonlicense "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/remotecluster"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/stackmon"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	defaultRequeue = reconciler.ReconciliationState{Result: controller.Result{Requeue: true, RequeueAfter: 10 * time.Second}}
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

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(ctx, d.Client, d.ES); err != nil {
		return results.WithError(err)
	}

	if err := configmap.ReconcileScriptsConfigMap(ctx, d.Client, d.ES); err != nil {
		return results.WithError(err)
	}

	_, err := common.ReconcileService(ctx, d.Client, services.NewTransportService(d.ES), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	externalService, err := common.ReconcileService(ctx, d.Client, services.NewExternalService(d.ES), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	var internalService *corev1.Service
	internalService, err = common.ReconcileService(ctx, d.Client, services.NewInternalService(d.ES), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	resourcesState, err := reconcile.NewResourcesStateFromAPI(d.Client, d.ES)
	if err != nil {
		return results.WithError(err)
	}

	warnUnsupportedDistro(resourcesState.AllPods, d.ReconcileState.Recorder)

	controllerUser, err := user.ReconcileUsersAndRoles(ctx, d.Client, d.ES, d.DynamicWatches(), d.Recorder())
	if err != nil {
		return results.WithError(err)
	}

	trustedHTTPCertificates, res := certificates.ReconcileHTTP(
		ctx,
		d,
		d.ES,
		[]corev1.Service{*externalService, *internalService},
		d.OperatorParameters.CACertRotation,
		d.OperatorParameters.CertRotation,
	)
	results.WithResults(res)
	if res != nil && res.HasError() {
		return results
	}

	// start the ES observer
	min, err := version.MinInPods(resourcesState.CurrentPods, label.VersionLabelName)
	if err != nil {
		return results.WithError(err)
	}
	if min == nil {
		min = &d.Version
	}
	observedState := d.Observers.ObservedStateResolver(
		d.ES,
		d.newElasticsearchClient(
			resourcesState,
			controllerUser,
			*min,
			trustedHTTPCertificates,
		),
	)

	// Always update the Elasticsearch state bits with the latest observed state.
	d.ReconcileState.
		UpdateClusterHealth(observedState()).    // Elasticsearch cluster health
		UpdateAvailableNodes(*resourcesState).   // Available nodes
		UpdateMinRunningVersion(*resourcesState) // Min running version

	res = certificates.ReconcileTransport(
		ctx,
		d,
		d.ES,
		d.OperatorParameters.CACertRotation,
		d.OperatorParameters.CertRotation,
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
		resourcesState,
		controllerUser,
		*min,
		trustedHTTPCertificates,
	)
	defer esClient.Close()

	esReachable, err := services.IsServiceReady(d.Client, *internalService)
	if err != nil {
		return results.WithError(err)
	}
	if esReachable {
		d.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionTrue, fmt.Sprintf("Service %s/%s has endpoints", internalService.Namespace, internalService.Name))
	} else {
		d.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, fmt.Sprintf("Service %s/%s has no endpoint", internalService.Namespace, internalService.Name))
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

	// reconcile the Elasticsearch license
	if esReachable {
		err = license.Reconcile(ctx, d.Client, d.ES, esClient, currentLicense)
		if err != nil {
			msg := "Could not reconcile cluster license, re-queuing"
			log.Info(msg, "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
			d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
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
	if err := settings.UpdateSeedHostsConfigMap(ctx, d.Client, d.ES, resourcesState.AllPods); err != nil {
		return results.WithError(err)
	}

	// setup a keystore with secure settings in an init container, if specified by the user
	keystoreResources, err := keystore.ReconcileResources(
		d,
		&d.ES,
		esv1.ESNamer,
		label.NewLabels(k8s.ExtractNamespacedName(&d.ES)),
		initcontainer.KeystoreParams,
	)
	if err != nil {
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
	err = stackmon.ReconcileConfigSecrets(d.Client, d.ES)
	if err != nil {
		return results.WithError(err)
	}

	// requeue if associations are defined but not yet configured, otherwise we may be in a situation where we deploy
	// Elasticsearch Pods once, then change their spec a few seconds later once the association is configured
	areAssocsConfigured, err := association.AreConfiguredIfSet(d.ES.GetAssociations(), d.Recorder())
	if err != nil {
		return results.WithError(err)
	}
	if !areAssocsConfigured {
		results.WithReconciliationState(defaultRequeue.WithReason("Some associations are not reconciled"))
	}

	// we want to reconcile suspended Pods before we start reconciling node specs as this is considered a debugging and
	// troubleshooting tool that does not follow the change budget restrictions
	if err := reconcileSuspendedPods(d.Client, d.ES, d.Expectations); err != nil {
		return results.WithError(err)
	}

	// reconcile StatefulSets and nodes configuration
	return results.WithResults(d.reconcileNodeSpecs(ctx, esReachable, esClient, d.ReconcileState, *resourcesState, keystoreResources))
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(
	state *reconcile.ResourcesState,
	user esclient.BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) esclient.Client {
	url := services.ElasticsearchURL(d.ES, state.CurrentPodsByPhase[corev1.PodRunning])
	return esclient.NewElasticsearchClient(
		d.OperatorParameters.Dialer,
		k8s.ExtractNamespacedName(&d.ES),
		url,
		user,
		v,
		caCerts,
		esclient.Timeout(d.ES),
	)
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
