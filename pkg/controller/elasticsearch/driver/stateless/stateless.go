// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateless

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/api"
	drivercommon "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

type statelessDriver struct {
	api.DefaultDriverParameters
}

func (sd *statelessDriver) K8sClient() k8s.Client {
	return sd.Client
}

func (sd *statelessDriver) DynamicWatches() watches.DynamicWatches {
	return sd.DefaultDriverParameters.DynamicWatches
}

func (sd *statelessDriver) Recorder() record.EventRecorder {
	return sd.DefaultDriverParameters.Recorder
}

// NewDriver returns the stateful driver implementation.
func NewDriver(parameters api.DefaultDriverParameters) api.Driver {
	return &statelessDriver{DefaultDriverParameters: parameters}
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources for a stateless Elasticsearch cluster.
func (sd *statelessDriver) Reconcile(ctx context.Context) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	log := ulog.FromContext(ctx).WithValues("is_stateless", true)
	log.Info("Reconciling stateless Elasticsearch cluster")

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(ctx, sd.Client, sd.ES); err != nil {
		return results.WithError(err)
	}

	// extract the metadata that should be propagated to children
	meta := metadata.Propagate(&sd.ES, metadata.Metadata{Labels: label.NewLabels(k8s.ExtractNamespacedName(&sd.ES))})

	if err := configmap.ReconcileScriptsConfigMap(ctx, sd.Client, sd.ES, meta); err != nil {
		return results.WithError(err)
	}

	externalService, err := common.ReconcileService(ctx, sd.Client, services.NewExternalService(sd.ES, meta), &sd.ES)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return results.WithReconciliationState(reconciler.Requeue.WithReason(fmt.Sprintf("Pending %s service recreation", services.ExternalServiceName(sd.ES.Name))))
		}
		return results.WithError(err)
	}

	// reconcile the internal service
	internalService, err := common.ReconcileService(ctx, sd.Client, services.NewInternalService(sd.ES, meta), &sd.ES)
	if err != nil {
		return results.WithError(err)
	}

	resourcesState, err := reconcile.NewResourcesStateFromAPI(sd.Client, sd.ES)
	if err != nil {
		return results.WithError(err)
	}

	drivercommon.WarnUnsupportedDistro(resourcesState.AllPods, sd.ReconcileState.Recorder)

	controllerUser, err := user.ReconcileUsersAndRoles(
		ctx,
		sd.Client,
		sd.ES,
		sd.DynamicWatches(),
		sd.Recorder(),
		sd.OperatorParameters.PasswordHasher,
		sd.OperatorParameters.PasswordGenerator,
		meta)
	if err != nil {
		return results.WithError(err)
	}

	trustedHTTPCertificates, res := certificates.ReconcileHTTP(
		ctx,
		sd,
		sd.ES,
		[]corev1.Service{*externalService, *internalService},
		sd.OperatorParameters.GlobalCA,
		sd.OperatorParameters.CACertRotation,
		sd.OperatorParameters.CertRotation,
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
		minVersion = &sd.Version
	}

	urlProvider := esclient.NewStaticURLProvider(services.InternalServiceURL(sd.ES))
	hasEndpoints := urlProvider.HasEndpoints()

	observedState := sd.Observers.ObservedStateResolver(
		ctx,
		sd.ES,
		drivercommon.ElasticsearchClientProvider(
			ctx,
			&sd.ES,
			sd.OperatorParameters.Dialer,
			urlProvider,
			controllerUser,
			*minVersion,
			trustedHTTPCertificates,
		),
		hasEndpoints,
	)

	// Always update the Elasticsearch state bits with the latest observed state.
	sd.ReconcileState.
		UpdateClusterHealth(observedState()). // Elasticsearch cluster health
		UpdateAvailableNodes(*resourcesState). // Available nodes
		UpdateMinRunningVersion(ctx, *resourcesState) // Min running version

	res = certificates.ReconcileTransport(
		ctx,
		sd,
		sd.ES,
		sd.OperatorParameters.GlobalCA,
		sd.OperatorParameters.CACertRotation,
		sd.OperatorParameters.CertRotation,
		meta,
	)
	results.WithResults(res)
	if res != nil && res.HasError() {
		return results
	}

	esClient := drivercommon.NewElasticsearchClient(
		ctx,
		&sd.ES,
		sd.OperatorParameters.Dialer,
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
		sd.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionTrue, reconcile.EsReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	} else {
		sd.ReconcileState.ReportCondition(esv1.ElasticsearchIsReachable, corev1.ConditionFalse, reconcile.EsReachableConditionMessage(internalService, hasEndpoints, hasKnownHealthState))
	}

	// TODO: handle license reconciliation errors properly

	// Compute seed hosts based on current masters with a podIP
	if err := settings.UpdateSeedHostsConfigMap(ctx, sd.Client, sd.ES, resourcesState.AllPods, meta); err != nil {
		return results.WithError(err)
	}

	// reconcile an empty File based settings Secret if it doesn't exist
	if err := filesettings.ReconcileEmptyFileSettingsSecret(ctx, sd.Client, sd.ES, true); err != nil {
		return results.WithError(err)
	}

	/* No keystore  init container in stateless mode, we use secure file settings instead */

	// set an annotation with the ClusterUUID, if bootstrapped
	requeue, err := bootstrap.ReconcileClusterUUID(ctx, sd.Client, &sd.ES, esClient, esReachable)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results = results.WithReconciliationState(reconciler.Requeue.WithReason("Elasticsearch cluster UUID is not reconciled"))
	}

	// requeue if associations are defined but not yet configured, otherwise we may be in a situation where we deploy
	// Elasticsearch Pods once, then change their spec a few seconds later once the association is configured
	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, sd.ES.GetAssociations(), sd.Recorder())
	if err != nil {
		return results.WithError(err)
	}
	if !areAssocsConfigured {
		results.WithReconciliationState(reconciler.Requeue.WithReason("Some associations are not reconciled"))
	}

	// reconcile CloneSets and nodes configuration
	return results.WithResults(sd.reconcileTiers(ctx, sd.Expectations, meta))
}
