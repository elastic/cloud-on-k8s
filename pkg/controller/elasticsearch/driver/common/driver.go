// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// Reconcile performs the reconciliation of the resources used by both stateless and stateful Elasticsearch clusters.
func (d *DefaultDriverParameters) Reconcile(ctx context.Context) *DefaultDriverParametersResult {
	results := &DefaultDriverParametersResult{
		Results: reconciler.NewResult(ctx),
	}
	log := ulog.FromContext(ctx).WithValues("is_stateless", d.ES.IsStateless())
	log.Info("Reconciling Elasticsearch cluster")

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(ctx, d.Client, d.ES); err != nil {
		return results.WithError(err)
	}

	// extract the metadata that should be propagated to children
	results.Meta = metadata.Propagate(&d.ES, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&d.ES))})

	if err := configmap.ReconcileScriptsConfigMap(ctx, d.Client, d.ES, results.Meta); err != nil {
		return results.WithError(err)
	}

	_, err := common.ReconcileService(ctx, d.Client, services.NewTransportService(d.ES, results.Meta), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	externalService, err := common.ReconcileService(ctx, d.Client, services.NewExternalService(d.ES, results.Meta), &d.ES)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return results.WithReconciliationState(reconciler.Requeue.WithReason(fmt.Sprintf("Pending %s service recreation", services.ExternalServiceName(d.ES.Name))))
		}
		return results.WithError(err)
	}

	var internalService *corev1.Service
	internalService, err = common.ReconcileService(ctx, d.Client, services.NewInternalService(d.ES, results.Meta), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	resourcesState, err := reconcile.NewResourcesStateFromAPI(d.Client, d.ES)
	if err != nil {
		return results.WithError(err)
	}
	results.ResourcesState = resourcesState

	WarnUnsupportedDistro(resourcesState.AllPods, d.ReconcileState.Recorder)

	controllerUser, err := user.ReconcileUsersAndRoles(
		ctx,
		d.Client,
		d.ES,
		d.dynamicWatches,
		d.recorder,
		d.OperatorParameters.PasswordHasher,
		d.OperatorParameters.PasswordGenerator,
		results.Meta)
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
		results.Meta,
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

	var urlProvider esclient.URLProvider
	if d.ES.IsStateless() {
		urlProvider = esclient.NewStaticURLProvider(services.InternalServiceURL(d.ES))
	} else {
		urlProvider = services.NewElasticsearchURLProvider(d.ES, d.Client)
	}

	hasEndpoints := urlProvider.HasEndpoints()

	observedState := d.Observers.ObservedStateResolver(
		ctx,
		d.ES,
		ElasticsearchClientProvider(
			ctx,
			&d.ES,
			d.OperatorParameters.Dialer,
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
		results.Meta,
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
	results.EsClient = NewElasticsearchClient(
		ctx,
		&d.ES,
		d.OperatorParameters.Dialer,
		urlProvider,
		controllerUser,
		*minVersion,
		trustedHTTPCertificates,
	)

	// use unknown health as a proxy for a cluster not responding to requests
	hasKnownHealthState := observedState() != esv1.ElasticsearchUnknownHealth
	results.EsReachable = hasEndpoints && hasKnownHealthState
	// report condition in Pod status
	if results.EsReachable {
		d.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionTrue, reconcile.EsReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	} else {
		d.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, reconcile.EsReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	}

	var currentLicense esclient.License
	if results.EsReachable {
		currentLicense, err = license.CheckElasticsearchLicense(ctx, results.EsClient)
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
			results.EsReachable = e.EsReachable
		}
		if err != nil {
			msg := "Could not verify license, re-queuing"
			log.Info(msg, "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
			d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
			results.WithReconciliationState(reconciler.Requeue.WithReason(msg))
		}
	}

	// reconcile the Elasticsearch license (even if we assume the cluster might not respond to requests to cover the case of
	// expired licenses where all health API responses are 403)
	if hasEndpoints {
		err = license.Reconcile(ctx, d.Client, d.ES, results.EsClient, currentLicense)
		if err != nil {
			msg := "Could not reconcile cluster license, re-queuing"
			// only log an event if Elasticsearch is in a state where success of this API call can be expected. The API call itself
			// will be logged by the client
			if hasKnownHealthState {
				log.Info(msg, "err", err, "namespace", d.ES.Namespace, "es_name", d.ES.Name)
				d.ReconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s", msg, err.Error()))
			}
			results.WithReconciliationState(reconciler.Requeue.WithReason(msg))
		}
	}

	// Compute seed hosts based on current masters with a podIP
	if err := settings.UpdateSeedHostsConfigMap(ctx, d.Client, d.ES, resourcesState.AllPods, results.Meta); err != nil {
		return results.WithError(err)
	}

	// reconcile an empty File based settings Secret if it doesn't exist
	if d.Version.GTE(filesettings.FileBasedSettingsMinPreVersion) {
		err = filesettings.ReconcileEmptyFileSettingsSecret(ctx, d.Client, d.ES, true)
		if err != nil {
			return results.WithError(err)
		}
	}

	// set an annotation with the ClusterUUID, if bootstrapped
	requeue, err := bootstrap.ReconcileClusterUUID(ctx, d.Client, &d.ES, results.EsClient, results.EsReachable)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results = results.WithReconciliationState(reconciler.Requeue.WithReason("Elasticsearch cluster UUID is not reconciled"))
	}

	// requeue if associations are defined but not yet configured, otherwise we may be in a situation where we deploy
	// Elasticsearch Pods once, then change their spec a few seconds later once the association is configured
	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, d.ES.GetAssociations(), d.Recorder())
	if err != nil {
		return results.WithError(err)
	}
	if !areAssocsConfigured {
		results.WithReconciliationState(reconciler.Requeue.WithReason("Some associations are not reconciled"))
	}

	return results
}
