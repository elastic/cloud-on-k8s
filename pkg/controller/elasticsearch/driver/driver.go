// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"crypto/x509"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/pdb"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	defaultRequeue = controller.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Driver orchestrates the reconciliation of an Elasticsearch resource.
// Its lifecycle is bound to a single reconciliation attempt.
type Driver interface {
	Reconcile() *reconciler.Results
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
	ES v1alpha1.Elasticsearch
	// SupportedVersions verifies whether we can support upgrading from the current pods.
	SupportedVersions esversion.LowestHighestSupportedVersions

	// Version is the version of Elasticsearch we want to reconcile towards.
	Version version.Version
	// Client is used to access the Kubernetes API.
	Client   k8s.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// State holds the accumulated state during the reconcile loop
	ReconcileState *reconcile.State
	// Observers that observe es clusters state.
	Observers *observer.Manager
	// DynamicWatches are handles to currently registered dynamic watches.
	DynamicWatches watches.DynamicWatches
	// Expectations control some expectations set on resources in the cache, in order to
	// avoid doing certain operations if the cache hasn't seen an up-to-date resource yet.
	Expectations *reconciler.Expectations
}

// defaultDriver is the default Driver implementation
type defaultDriver struct {
	DefaultDriverParameters
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *defaultDriver) Reconcile() *reconciler.Results {
	results := &reconciler.Results{}

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(d.Client, d.ES); err != nil {
		return results.WithError(err)
	}

	if err := configmap.ReconcileScriptsConfigMap(d.Client, d.Scheme, d.ES); err != nil {
		return results.WithError(err)
	}

	externalService, err := common.ReconcileService(d.Client, d.Scheme, services.NewExternalService(d.ES), &d.ES)
	if err != nil {
		return results.WithError(err)
	}

	certificateResources, res := certificates.Reconcile(
		d.Client,
		d.Scheme,
		d.DynamicWatches,
		d.ES,
		[]corev1.Service{*externalService},
		d.OperatorParameters.CACertRotation,
		d.OperatorParameters.CertRotation,
	)
	if results.WithResults(res).HasError() {
		return results
	}

	internalUsers, err := user.ReconcileUsers(d.Client, d.Scheme, d.ES)
	if err != nil {
		return results.WithError(err)
	}

	resourcesState, err := reconcile.NewResourcesStateFromAPI(d.Client, d.ES)
	if err != nil {
		return results.WithError(err)
	}
	min, err := esversion.MinVersion(resourcesState.CurrentPods)
	if err != nil {
		return results.WithError(err)
	}
	if min == nil {
		min = &d.Version
	}

	warnUnsupportedDistro(resourcesState.AllPods, d.ReconcileState.Recorder)

	observedState := d.Observers.ObservedStateResolver(
		k8s.ExtractNamespacedName(&d.ES),
		d.newElasticsearchClient(
			resourcesState,
			internalUsers.ControllerUser,
			*min,
			certificateResources.TrustedHTTPCertificates,
		))

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		d.ReconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	if err := pdb.Reconcile(d.Client, d.Scheme, d.ES); err != nil {
		return results.WithError(err)
	}

	if err := d.SupportedVersions.VerifySupportsExistingPods(resourcesState.CurrentPods); err != nil {
		return results.WithError(err)
	}

	// TODO: support user-supplied certificate (non-ca)
	esClient := d.newElasticsearchClient(
		resourcesState,
		internalUsers.ControllerUser,
		*min,
		certificateResources.TrustedHTTPCertificates,
	)
	defer esClient.Close()

	esReachable, err := services.IsServiceReady(d.Client, *externalService)
	if err != nil {
		return results.WithError(err)
	}

	results.Apply(
		"reconcile-cluster-license",
		func() (controller.Result, error) {
			err := license.Reconcile(
				d.Client,
				d.ES,
				esClient,
				observedState.ClusterLicense,
			)
			if err != nil && esReachable {
				d.ReconcileState.AddEvent(
					corev1.EventTypeWarning,
					events.EventReasonUnexpected,
					fmt.Sprintf("Could not update cluster license: %s", err.Error()),
				)
				return defaultRequeue, err
			}
			return controller.Result{}, err
		},
	)

	// Compute seed hosts based on current masters with a podIP
	if err := settings.UpdateSeedHostsConfigMap(d.Client, d.Scheme, d.ES, resourcesState.AllPods); err != nil {
		return results.WithError(err)
	}

	// setup a keystore with secure settings in an init container, if specified by the user
	keystoreResources, err := keystore.NewResources(
		d.Client,
		d.Recorder,
		d.DynamicWatches,
		&d.ES,
		initcontainer.KeystoreParams,
	)
	if err != nil {
		return results.WithError(err)
	}

	res = d.reconcileNodeSpecs(esReachable, esClient, d.ReconcileState, observedState, *resourcesState, keystoreResources)
	if results.WithResults(res).HasError() {
		return results
	}

	d.ReconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	return results
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(
	state *reconcile.ResourcesState,
	user user.User,
	v version.Version,
	caCerts []*x509.Certificate,
) esclient.Client {
	url := services.ElasticsearchURL(d.ES, state.CurrentPodsByPhase[corev1.PodRunning])
	return esclient.NewElasticsearchClient(d.OperatorParameters.Dialer, url, user.Auth(), v, caCerts)
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
