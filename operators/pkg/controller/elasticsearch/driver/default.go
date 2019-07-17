// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"crypto/x509"
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controller "sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/migration"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/cleanup"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/configmap"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
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
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/version/version6"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

// initContainerParams is used to generate the init container that will load the secure settings into a keystore
var initContainerParams = keystore.InitContainerParameters{
	KeystoreCreateCommand:         "/usr/share/elasticsearch/bin/elasticsearch-keystore create",
	KeystoreAddCommand:            "/usr/share/elasticsearch/bin/elasticsearch-keystore add",
	SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
	DataVolumePath:                esvolume.ElasticsearchDataMountPath,
}

// defaultDriver is the default Driver implementation
type defaultDriver struct {
	// Options are the options that the driver was created with.
	Options

	expectations *Expectations

	// supportedVersions verifies whether we can support upgrading from the current pods.
	supportedVersions esversion.LowestHighestSupportedVersions

	// usersReconciler reconciles external and internal users and returns the current internal users.
	usersReconciler func(
		c k8s.Client,
		scheme *runtime.Scheme,
		es v1alpha1.Elasticsearch,
	) (*user.InternalUsers, error)

	// expectedPodsAndResourcesResolver returns a list of pod specs with context that we would expect to find in the
	// Elasticsearch cluster.
	//
	// paramsTmpl argument is a partially filled NewPodSpecParams (TODO: refactor into its own params struct)
	expectedPodsAndResourcesResolver func(
		es v1alpha1.Elasticsearch,
		paramsTmpl pod.NewPodSpecParams,
	) ([]pod.PodSpecContext, error)

	// observedStateResolver resolves the currently observed state of Elasticsearch from the ES API
	observedStateResolver func(clusterName types.NamespacedName, esClient esclient.Client) observer.State

	// resourcesStateResolver resolves the current state of the K8s resources from the K8s API
	resourcesStateResolver func(
		c k8s.Client,
		es v1alpha1.Elasticsearch,
	) (*reconcile.ResourcesState, error)

	// clusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set where relevant
	// this can safely be set to nil when it's not relevant (e.g for ES <= 6)
	clusterInitialMasterNodesEnforcer func(
		performableChanges mutation.PerformableChanges,
		resourcesState reconcile.ResourcesState,
	) (*mutation.PerformableChanges, error)

	// zen1SettingsUpdater updates the zen1 settings for the current pods.
	// this can safely be set to nil when it's not relevant (e.g when all nodes in the cluster is >= 7)
	zen1SettingsUpdater func(
		cluster v1alpha1.Elasticsearch,
		c k8s.Client,
		esClient esclient.Client,
		allPods []corev1.Pod,
		performableChanges *mutation.PerformableChanges,
		reconcileState *reconcile.State,
	) (bool, error)

	// zen2SettingsUpdater updates the zen2 settings for the current changes.
	// this can safely be set to nil when it's not relevant (e.g when all nodes in the cluster is <7)
	zen2SettingsUpdater func(
		esClient esclient.Client,
		minVersion version.Version,
		changes mutation.Changes,
		performableChanges mutation.PerformableChanges,
	) error

	// TODO: implement
	// // apiObjectsGarbageCollector garbage collects API objects for older versions once they are no longer needed.
	// apiObjectsGarbageCollector func(
	// 	c k8s.Client,
	// 	es v1alpha1.Elasticsearch,
	// 	version version.Version,
	// 	state mutation.PodsState,
	// ) (reconcile.Result, error) // could get away with one impl
}

// Reconcile fulfills the Driver interface and reconciles the cluster resources.
func (d *defaultDriver) Reconcile(
	es v1alpha1.Elasticsearch,
	reconcileState *reconcile.State,
) *reconciler.Results {
	results := &reconciler.Results{}

	// garbage collect secrets attached to this cluster that we don't need anymore
	if err := cleanup.DeleteOrphanedSecrets(d.Client, es); err != nil {
		return results.WithError(err)
	}

	if err := reconcileScriptsConfigMap(d.Client, d.Scheme, es); err != nil {
		return results.WithError(err)
	}

	genericResources, res := reconcileGenericResources(
		d.Client,
		d.Scheme,
		es,
	)
	if results.WithResults(res).HasError() {
		return results
	}

	certificateResources, res := certificates.Reconcile(
		d.Client,
		d.Scheme,
		d.DynamicWatches,
		es,
		[]corev1.Service{genericResources.ExternalService},
		d.Parameters.CACertRotation,
		d.Parameters.CertRotation,
	)
	if results.WithResults(res).HasError() {
		return results
	}

	internalUsers, err := d.usersReconciler(d.Client, d.Scheme, es)
	if err != nil {
		return results.WithError(err)
	}

	resourcesState, err := d.resourcesStateResolver(d.Client, es)
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

	warnUnsupportedDistro(resourcesState.AllPods, reconcileState.Recorder)

	observedState := d.observedStateResolver(
		k8s.ExtractNamespacedName(&es),
		d.newElasticsearchClient(
			genericResources.ExternalService,
			internalUsers.ControllerUser,
			*min,
			certificateResources.TrustedHTTPCertificates,
		))

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	if err := pdb.Reconcile(d.Client, d.Scheme, es); err != nil {
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
				es,
				esClient,
				observedState.ClusterLicense,
			)
			if err != nil && esReachable {
				reconcileState.AddEvent(
					corev1.EventTypeWarning,
					events.EventReasonUnexpected,
					fmt.Sprintf("Could not update cluster license: %s", err.Error()),
				)
				return defaultRequeue, err
			}
			return controller.Result{}, err
		},
	)
	//
	//if d.clusterInitialMasterNodesEnforcer != nil {
	//	performableChanges, err = d.clusterInitialMasterNodesEnforcer(*performableChanges, *resourcesState)
	//	if err != nil {
	//		return results.WithError(err)
	//	}
	//}

	// Compute seed hosts based on current masters with a podIP
	if err := settings.UpdateSeedHostsConfigMap(d.Client, d.Scheme, es, resourcesState.AllPods); err != nil {
		return results.WithError(err)
	}

	// setup a keystore with secure settings in an init container, if specified by the user
	keystoreResources, err := keystore.NewResources(
		d.Client,
		d.Recorder,
		d.DynamicWatches,
		&es,
		initContainerParams,
	)
	if err != nil {
		return results.WithError(err)
	}

	// TODO: this is a mess, refactor and unit test correctly
	podTemplateSpecBuilder := func(nodeSpec v1alpha1.NodeSpec, cfg settings.CanonicalConfig) (corev1.PodTemplateSpec, error) {
		return esversion.BuildPodTemplateSpec(
			es,
			nodeSpec,
			pod.NewPodSpecParams{
				ProbeUser: internalUsers.ProbeUser.Auth(),
				UnicastHostsVolume: volume.NewConfigMapVolume(
					name.UnicastHostsConfigMap(es.Name), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
				),
				UsersSecretVolume: volume.NewSecretVolumeWithMountPath(
					user.XPackFileRealmSecretName(es.Name),
					esvolume.XPackFileRealmVolumeName,
					esvolume.XPackFileRealmVolumeMountPath,
				),
				KeystoreResources: keystoreResources,
			},
			cfg,
			version6.NewEnvironmentVars,
			initcontainer.NewInitContainers,
		)
	}

	res = d.reconcileNodeSpecs(es, esReachable, podTemplateSpecBuilder, esClient, observedState)
	if results.WithResults(res).HasError() {
		return results
	}

	//
	//// Call Zen1 setting updater before new masters are created to ensure that they immediately start with the
	//// correct value for minimum_master_nodes.
	//// For instance if a 3 master nodes cluster is updated and a grow-and-shrink strategy of one node is applied then
	//// minimum_master_nodes is increased from 2 to 3 for new and current nodes.
	//if d.zen1SettingsUpdater != nil {
	//	requeue, err := d.zen1SettingsUpdater(
	//		es,
	//		d.Client,
	//		esClient,
	//		resourcesState.AllPods,
	//		performableChanges,
	//		reconcileState,
	//	)
	//
	//	if err != nil {
	//		return results.WithError(err)
	//	}
	//
	//	if requeue {
	//		results.WithResult(defaultRequeue)
	//	}
	//}

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES external service not ready yet for shard migration reconciliation. Requeuing.", "namespace", es.Namespace, "es_name", es.Name)

		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods.Pods())

		return results.WithResult(defaultRequeue)
	}
	//
	//if d.zen2SettingsUpdater != nil {
	//	// TODO: would prefer to do this after MigrateData iff there's no changes? or is that an premature optimization?
	//	if err := d.zen2SettingsUpdater(
	//		esClient,
	//		*min,
	//		*changes,
	//		*performableChanges,
	//	); err != nil {
	//		return results.WithResult(defaultRequeue).WithError(err)
	//	}
	//}

	reconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	return results
}

func (d *defaultDriver) reconcileNodeSpecs(
	es v1alpha1.Elasticsearch,
	esReachable bool,
	podSpecBuilder esversion.PodTemplateSpecBuilder,
	esClient esclient.Client,
	observedState observer.State,
) *reconciler.Results {
	results := &reconciler.Results{}

	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return results.WithError(err)
	}

	nodeSpecResources, err := nodespec.BuildExpectedResources(es, podSpecBuilder)
	if err != nil {
		return results.WithError(err)
	}

	// TODO: handle zen2 initial master nodes more cleanly
	//  should be empty once cluster is bootstraped
	var initialMasters []string
	// TODO: refactor/move
	for _, res := range nodeSpecResources {
		cfg, err := res.Config.Unpack()
		if err != nil {
			return results.WithError(err)
		}
		if cfg.Node.Master {
			for i := 0; i < int(*res.StatefulSet.Spec.Replicas); i++ {
				initialMasters = append(initialMasters, fmt.Sprintf("%s-%d", res.StatefulSet.Name, i))
			}
		}
	}
	for i := range nodeSpecResources {
		if err := nodeSpecResources[i].Config.SetStrings(settings.ClusterInitialMasterNodes, initialMasters...); err != nil {
			return results.WithError(err)
		}
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
		if err := settings.ReconcileConfig(d.Client, es, nodeSpecRes.StatefulSet.Name, nodeSpecRes.Config); err != nil {
			return results.WithError(err)
		}
		if _, err := common.ReconcileService(d.Client, d.Scheme, &nodeSpecRes.HeadlessService, &es); err != nil {
			return results.WithError(err)
		}
		ssetToApply := *nodeSpecRes.StatefulSet.DeepCopy()
		actual, exists := actualStatefulSets.GetByName(ssetToApply.Name)
		if exists && sset.Replicas(ssetToApply) < sset.Replicas(actual) {
			// sset needs to be scaled down
			// update the sset to use the new spec but don't scale replicas down for now
			ssetToApply.Spec.Replicas = actual.Spec.Replicas
		}
		if err := sset.ReconcileStatefulSet(d.Client, d.Scheme, es, ssetToApply); err != nil {
			return results.WithError(err)
		}
	}

	if !esReachable {
		// cannot perform downscale or rolling upgrade if we cannot request Elasticsearch
		return results.WithResult(defaultRequeue)
	}

	// Phase 2: handle sset scale down.
	// We want to safely remove nodes from the cluster, either because the sset requires less replicas,
	// or because it should be removed entirely.
	for i, actual := range actualStatefulSets {
		expected, shouldExist := nodeSpecResources.StatefulSets().GetByName(actual.Name)
		switch {
		// stateful set removal
		case !shouldExist:
			target := int32(0)
			removalResult := d.scaleStatefulSetDown(&actualStatefulSets[i], target, esClient, observedState)
			results.WithResults(removalResult)
			if removalResult.HasError() {
				return results
			}
		// stateful set downscale
		case actual.Spec.Replicas != nil && sset.Replicas(expected) < sset.Replicas(actual):
			target := sset.Replicas(expected)
			downscaleResult := d.scaleStatefulSetDown(&actualStatefulSets[i], target, esClient, observedState)
			if downscaleResult.HasError() {
				return results
			}
		}
	}

	// Phase 3: handle rolling upgrades.
	// Control nodes restart (upgrade) by manually decrementing rollingUpdate.Partition.
	rollingUpgradesRes := d.handleRollingUpgrades(es, esClient, actualStatefulSets)
	results.WithResults(rollingUpgradesRes)
	if rollingUpgradesRes.HasError() {
		return results
	}

	// TODO:
	//  - change budget
	//  - zen1, zen2
	return results
}

func (d *defaultDriver) scaleStatefulSetDown(
	statefulSet *appsv1.StatefulSet,
	targetReplicas int32,
	esClient esclient.Client,
	observedState observer.State,
) *reconciler.Results {
	results := &reconciler.Results{}
	logger := log.WithValues("statefulset", k8s.ExtractNamespacedName(statefulSet))

	if sset.Replicas(*statefulSet) == 0 && targetReplicas == 0 {
		// we don't expect any new replicas in this statefulset, remove it
		logger.Info("Deleting statefulset", "namespace", statefulSet.Namespace, "name", statefulSet.Name)
		if err := d.Client.Delete(statefulSet); err != nil {
			return results.WithError(err)
		}
	}
	// copy the current replicas, to be decremented with nodes to remove
	initialReplicas := sset.Replicas(*statefulSet)
	updatedReplicas := initialReplicas

	// leaving nodes names can be built from StatefulSet name and ordinals
	// nodes are ordered by highest ordinal first
	var leavingNodes []string
	for i := initialReplicas - 1; i > targetReplicas-1; i-- {
		leavingNodes = append(leavingNodes, sset.PodName(statefulSet.Name, i))
	}

	// TODO: don't remove last master/last data nodes?
	// TODO: detect cases where data migration cannot happen since no nodes to host shards?

	// migrate data away from these nodes before removing them
	logger.V(1).Info("Migrating data away from nodes", "nodes", leavingNodes)
	if err := migration.MigrateData(esClient, leavingNodes); err != nil {
		return results.WithError(err)
	}

	for _, node := range leavingNodes {
		if migration.IsMigratingData(observedState, node, leavingNodes) {
			// data migration not over yet: schedule a requeue
			logger.V(1).Info("Data migration not over yet, skipping node deletion", "node", node)
			results.WithResult(defaultRequeue)
			// no need to check other nodes since we remove them in order and this one isn't ready anyway
			break
		}
		// data migration over: allow pod to be removed
		updatedReplicas--
	}

	if updatedReplicas != initialReplicas {
		// update cluster coordination settings to account for nodes deletion
		// TODO: update zen1/zen2

		// trigger deletion of nodes whose data migration is over
		logger.V(1).Info("Scaling replicas down", "from", initialReplicas, "to", updatedReplicas)
		statefulSet.Spec.Replicas = &updatedReplicas
		if err := d.Client.Update(statefulSet); err != nil {
			return results.WithError(err)
		}
	}

	// TODO: clear allocation excludes

	return results
}

// newElasticsearchClient creates a new Elasticsearch HTTP client for this cluster using the provided user
func (d *defaultDriver) newElasticsearchClient(service corev1.Service, user user.User, v version.Version, caCerts []*x509.Certificate) esclient.Client {
	url := fmt.Sprintf("https://%s.%s.svc:%d", service.Name, service.Namespace, network.HTTPPort)
	return esclient.NewElasticsearchClient(d.Dialer, url, user.Auth(), v, caCerts)
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
