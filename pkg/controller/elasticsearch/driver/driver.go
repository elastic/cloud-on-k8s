// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonapm "github.com/elastic/cloud-on-k8s/pkg/controller/common/apm"
	commondriver "github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
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
	ES esv1.Elasticsearch
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
	Expectations *expectations.Expectations

	// Parent context, contains tracing data
	Context context.Context
}

// defaultDriver is the default Driver implementation
type defaultDriver struct {
	DefaultDriverParameters
}

func (d *defaultDriver) K8sClient() k8s.Client {
	return d.Client
}

func (d *defaultDriver) Scheme() *runtime.Scheme {
	return d.DefaultDriverParameters.Scheme
}

func (d *defaultDriver) DynamicWatches() watches.DynamicWatches {
	return d.DefaultDriverParameters.DynamicWatches
}

func (d *defaultDriver) Recorder() record.EventRecorder {
	return d.DefaultDriverParameters.Recorder
}

var _ commondriver.Interface = &defaultDriver{}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *defaultDriver) Reconcile() *reconciler.Results {
	results := reconciler.NewResult(d.Context)

	// garbage collect secrets attached to this cluster that we don't need anymore
	span, ctx := apm.StartSpan(d.Context, "delete_orphaned_secrets", "app")
	if err := cleanup.DeleteOrphanedSecrets(d.Client, d.ES); err != nil {
		return results.WithError(err)
	}
	span.End()

	span, _ = apm.StartSpan(d.Context, "reconcile_scripts", "app")
	if err := configmap.ReconcileScriptsConfigMap(d.Client, d.Scheme(), d.ES); err != nil {
		return results.WithError(err)
	}
	span.End()

	span, _ = apm.StartSpan(d.Context, "reconcile_service", "app")
	externalService, err := common.ReconcileService(d.Client, d.Scheme(), services.NewExternalService(d.ES), &d.ES)
	if err != nil {
		return results.WithError(err)
	}
	span.End()

	span, _ = apm.StartSpan(d.Context, "reconcile_certs", "app")
	certificateResources, res := certificates.Reconcile(
		d,
		d.ES,
		[]corev1.Service{*externalService},
		d.OperatorParameters.CACertRotation,
		d.OperatorParameters.CertRotation,
	)
	span.End()
	if results.WithResults(res).HasError() {
		return results
	}

	span, _ = apm.StartSpan(d.Context, "reconcile_users", "app")
	internalUsers, err := user.ReconcileUsers(d.Client, d.Scheme(), d.ES)
	if err != nil {
		return results.WithError(err)
	}
	span.End()

	resourcesState, err := reconcile.NewResourcesStateFromAPI(d.Client, d.ES)
	if err != nil {
		return results.WithError(err)
	}
	min, err := label.MinVersion(resourcesState.CurrentPods)
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
		),
		commonapm.TracerFromContext(d.Context))

	// always update the elasticsearch state bits
	d.ReconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	if err := d.verifySupportsExistingPods(resourcesState.CurrentPods); err != nil {
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

	span, ctx = apm.StartSpan(d.Context, "reconcile_license", "app")
	results.Apply(
		"reconcile-cluster-license",
		func() (controller.Result, error) {
			if !esReachable {
				return defaultRequeue, nil
			}

			err := license.Reconcile(
				ctx,
				d.Client,
				d.ES,
				esClient,
				observedState.ClusterLicense,
			)
			span.End()
			if err != nil {
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
	span, _ = apm.StartSpan(d.Context, "update_seed_hosts", "app")
	if err := settings.UpdateSeedHostsConfigMap(d.Client, d.Scheme(), d.ES, resourcesState.AllPods); err != nil {
		return results.WithError(err)
	}
	span.End()

	// setup a keystore with secure settings in an init container, if specified by the user
	span, _ = apm.StartSpan(d.Context, "update_keystore", "app")
	keystoreResources, err := keystore.NewResources(
		d,
		&d.ES,
		esv1.ESNamer,
		label.NewLabels(k8s.ExtractNamespacedName(&d.ES)),
		initcontainer.KeystoreParams,
	)
	span.End()
	if err != nil {
		return results.WithError(err)
	}

	// set an annotation with the ClusterUUID, if bootstrapped
	span, ctx = apm.StartSpan(d.Context, "reconcile_uuid", "app")
	requeue, err := bootstrap.ReconcileClusterUUID(ctx, d.Client, &d.ES, esClient, esReachable)
	span.End()
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results = results.WithResult(defaultRequeue)
	}

	// reconcile StatefulSets and nodes configuration
	span, ctx = apm.StartSpan(d.Context, "reconcile_node_specs", "app")
	res = d.reconcileNodeSpecs(ctx, esReachable, esClient, d.ReconcileState, observedState, *resourcesState, keystoreResources, certificateResources)
	span.End()
	results = results.WithResults(res)

	if res.HasError() {
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
