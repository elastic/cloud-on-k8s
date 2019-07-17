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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pdb"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	esversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/zen1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/zen2"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

// initContainerParams is used to generate the init container that will load the secure settings into a keystore
var initContainerParams = keystore.InitContainerParameters{
	KeystoreCreateCommand:         "/usr/share/elasticsearch/bin/elasticsearch-keystore create",
	KeystoreAddCommand:            "/usr/share/elasticsearch/bin/elasticsearch-keystore add",
	SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
	DataVolumePath:                esvolume.ElasticsearchDataMountPath,
}

var (
	log            = logf.Log.WithName("driver")
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
	// State holds the accumulated state during the reconcile loop
	ReconcileState *reconcile.State

	// Version is the version of Elasticsearch we want to reconcile towards.
	Version version.Version
	// Client is used to access the Kubernetes API.
	Client   k8s.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Observers that observe es clusters state.
	Observers *observer.Manager
	// DynamicWatches are handles to currently registered dynamic watches.
	DynamicWatches watches.DynamicWatches
	// Expectations control some expectations set on resources in the cache, in order to
	// avoid doing certain operations if the cache hasn't seen an up-to-date resource yet.
	Expectations *Expectations
	// supportedVersions verifies whether we can support upgrading from the current pods.
	supportedVersions esversion.LowestHighestSupportedVersions
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

	if err := reconcileScriptsConfigMap(d.Client, d.Scheme, d.ES); err != nil {
		return results.WithError(err)
	}

	genericResources, res := reconcileGenericResources(
		d.Client,
		d.Scheme,
		d.ES,
	)
	if results.WithResults(res).HasError() {
		return results
	}

	certificateResources, res := certificates.Reconcile(
		d.Client,
		d.Scheme,
		d.DynamicWatches,
		d.ES,
		[]corev1.Service{genericResources.ExternalService},
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
	min, err := esversion.MinVersion(resourcesState.CurrentPods.Pods())
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
			genericResources.ExternalService,
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

	if err := d.supportedVersions.VerifySupportsExistingPods(resourcesState.CurrentPods.Pods()); err != nil {
		return results.WithError(err)
	}

	// TODO: support user-supplied certificate (non-ca)
	esClient := d.newElasticsearchClient(
		genericResources.ExternalService,
		internalUsers.ControllerUser,
		*min,
		certificateResources.TrustedHTTPCertificates,
	)
	defer esClient.Close()

	esReachable, err := services.IsServiceReady(d.Client, genericResources.ExternalService)
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
		initContainerParams,
	)
	if err != nil {
		return results.WithError(err)
	}

	// TODO: this is a mess, refactor and unit test correctly
	podTemplateSpecBuilder := func(nodeSpec v1alpha1.NodeSpec, cfg settings.CanonicalConfig) (corev1.PodTemplateSpec, error) {
		return esversion.BuildPodTemplateSpec(
			d.ES,
			nodeSpec,
			pod.NewPodSpecParams{
				ProbeUser: internalUsers.ProbeUser.Auth(),
				UnicastHostsVolume: volume.NewConfigMapVolume(
					name.UnicastHostsConfigMap(d.ES.Name), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
				),
				UsersSecretVolume: volume.NewSecretVolumeWithMountPath(
					user.XPackFileRealmSecretName(d.ES.Name),
					esvolume.XPackFileRealmVolumeName,
					esvolume.XPackFileRealmVolumeMountPath,
				),
				KeystoreResources: keystoreResources,
			},
			cfg,
			zen1.NewEnvironmentVars,
			initcontainer.NewInitContainers,
		)
	}

	res = d.reconcileNodeSpecs(esReachable, podTemplateSpecBuilder, esClient, d.ReconcileState, observedState, *resourcesState)
	if results.WithResults(res).HasError() {
		return results
	}

	d.ReconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	return results
}

func (d *defaultDriver) reconcileNodeSpecs(
	esReachable bool,
	podSpecBuilder esversion.PodTemplateSpecBuilder,
	esClient esclient.Client,
	reconcileState *reconcile.State,
	observedState observer.State,
	resourcesState reconcile.ResourcesState,
) *reconciler.Results {
	results := &reconciler.Results{}

	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&d.ES))
	if err != nil {
		return results.WithError(err)
	}

	if !d.Expectations.GenerationExpected(actualStatefulSets.ObjectMetas()...) {
		// Our cache of StatefulSets is out of date compared to previous reconciliation operations.
		// This will probably lead to conflicting sset updates (which is ok), but also to
		// conflicting ES calls (set/reset zen1/zen2/allocation excludes, etc.), which may not be ok.
		log.V(1).Info("StatefulSet cache out-of-date, re-queueing", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		return results.WithResult(defaultRequeue)
	}

	nodeSpecResources, err := nodespec.BuildExpectedResources(d.ES, podSpecBuilder)
	if err != nil {
		return results.WithError(err)
	}

	// TODO: there is a split brain possibility here if going from 1 to 3 masters or 3 to 7.
	//  See https://github.com/elastic/cloud-on-k8s/issues/1281.

	// patch configs to consider zen1 minimum master nodes
	if err := zen1.SetupMinimumMasterNodesConfig(nodeSpecResources); err != nil {
		return results.WithError(err)
	}
	// patch configs to consider zen2 initial master nodes
	if err := zen2.SetupInitialMasterNodes(d.ES, observedState, d.Client, nodeSpecResources); err != nil {
		return results.WithError(err)
	}

	// Phase 1: apply expected StatefulSets resources, but don't scale down.
	// The goal is to:
	// 1. scale sset up (eg. go from 3 to 5 replicas).
	// 2. apply configuration changes on the sset resource, to be used for future pods creation/recreation,
	//    but do not rotate pods yet.
	// 3. do **not** apply replicas scale down, otherwise nodes would be deleted before
	//    we handle a clean deletion.
	for _, nodeSpecRes := range nodeSpecResources {
		// always reconcile config (will apply to new & recreated pods)
		if err := settings.ReconcileConfig(d.Client, d.ES, nodeSpecRes.StatefulSet.Name, nodeSpecRes.Config); err != nil {
			return results.WithError(err)
		}
		if _, err := common.ReconcileService(d.Client, d.Scheme, &nodeSpecRes.HeadlessService, &d.ES); err != nil {
			return results.WithError(err)
		}
		ssetToApply := *nodeSpecRes.StatefulSet.DeepCopy()
		actual, exists := actualStatefulSets.GetByName(ssetToApply.Name)
		if exists && sset.Replicas(ssetToApply) < sset.Replicas(actual) {
			// sset needs to be scaled down
			// update the sset to use the new spec but don't scale replicas down for now
			ssetToApply.Spec.Replicas = actual.Spec.Replicas
		}
		if err := sset.ReconcileStatefulSet(d.Client, d.Scheme, d.ES, ssetToApply); err != nil {
			return results.WithError(err)
		}
	}

	if !esReachable {
		// Cannot perform next operations if we cannot request Elasticsearch.
		log.Info("ES external service not ready yet for further reconciliation, re-queuing.", "namespace", d.ES.Namespace, "es_name", d.ES.Name)
		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods.Pods())
		return results.WithResult(defaultRequeue)
	}

	// Update Zen1 minimum master nodes through the API, corresponding to the current nodes we have.
	requeue, err := zen1.UpdateMinimumMasterNodes(d.Client, d.ES, esClient, actualStatefulSets, reconcileState)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithResult(defaultRequeue)
	}
	// Maybe clear zen2 voting config exclusions.
	requeue, err = zen2.ClearVotingConfigExclusions(d.ES, d.Client, esClient, actualStatefulSets)
	if err != nil {
		return results.WithError(err)
	}
	if requeue {
		results.WithResult(defaultRequeue)
	}

	// Phase 2: handle sset scale down.
	// We want to safely remove nodes from the cluster, either because the sset requires less replicas,
	// or because it should be removed entirely.
	downscaleRes := d.HandleDownscale(nodeSpecResources.StatefulSets(), actualStatefulSets, esClient, observedState, reconcileState)
	results.WithResults(downscaleRes)
	if downscaleRes.HasError() {
		return results
	}

	// Phase 3: handle rolling upgrades.
	// Control nodes restart (upgrade) by manually decrementing rollingUpdate.Partition.
	rollingUpgradesRes := d.handleRollingUpgrades(esClient, actualStatefulSets)
	results.WithResults(rollingUpgradesRes)
	if rollingUpgradesRes.HasError() {
		return results
	}

	// TODO:
	//  - change budget
	//  - grow and shrink
	return results
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(service corev1.Service, user user.User, v version.Version, caCerts []*x509.Certificate) esclient.Client {
	url := fmt.Sprintf("https://%s.%s.svc:%d", service.Name, service.Namespace, network.HTTPPort)
	return esclient.NewElasticsearchClient(d.OperatorParameters.Dialer, url, user.Auth(), v, caCerts)
}

func reconcileScriptsConfigMap(c k8s.Client, scheme *runtime.Scheme, es v1alpha1.Elasticsearch) error {
	fsScript, err := initcontainer.RenderPrepareFsScript()
	if err != nil {
		return err
	}

	scriptsConfigMap := configmap.NewConfigMapWithData(
		types.NamespacedName{Namespace: es.Namespace, Name: name.ScriptsConfigMap(es.Name)},
		map[string]string{
			pod.ReadinessProbeScriptConfigKey:      pod.ReadinessProbeScript,
			initcontainer.PrepareFsScriptConfigKey: fsScript,
		})

	if err := configmap.ReconcileConfigMap(c, scheme, es, scriptsConfigMap); err != nil {
		return err
	}

	return nil
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
