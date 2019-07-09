// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"crypto/x509"
	"fmt"

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

// defaultDriver is the default Driver implementation
type defaultDriver struct {
	// Options are the options that the driver was created with.
	Options

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
		operatorImage string,
	) ([]pod.PodSpecContext, error)

	// observedStateResolver resolves the currently observed state of Elasticsearch from the ES API
	observedStateResolver func(clusterName types.NamespacedName, caCerts []*x509.Certificate, esClient esclient.Client) observer.State

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

	if err := settings.ReconcileSecureSettings(d.Client, reconcileState.Recorder, d.Scheme, d.DynamicWatches, es); err != nil {
		return results.WithError(err)
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

	observedState := d.observedStateResolver(
		k8s.ExtractNamespacedName(&es),
		certificateResources.TrustedHTTPCertificates,
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

	//podsState := mutation.NewPodsState(*resourcesState, observedState)

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

	//
	//// There might be some ongoing creations and deletions our k8s client cache
	//// hasn't seen yet. In such case, requeue until we are in-sync.
	//// Otherwise, we could end up re-creating multiple times the same pod with
	//// different generated names through multiple reconciliation iterations.
	//if !d.PodsExpectations.Fulfilled(namespacedName) {
	//	log.Info("Pods creations and deletions expectations are not satisfied yet. Requeuing.")
	//	return results.WithResult(defaultRequeue)
	//}
	//
	//changes, err := d.calculateChanges(internalUsers, es, *resourcesState)
	//if err != nil {
	//	return results.WithError(err)
	//}
	//
	//log.Info(
	//	"Calculated all required changes",
	//	"to_create:", len(changes.ToCreate),
	//	"to_keep:", len(changes.ToKeep),
	//	"to_delete:", len(changes.ToDelete),
	//)
	//
	//// restart ES processes that need to be restarted before going on with other changes
	//done, err := restart.HandleESRestarts(
	//	restart.RestartContext{
	//		Cluster:        es,
	//		EventsRecorder: reconcileState.Recorder,
	//		K8sClient:      d.Client,
	//		Changes:        *changes,
	//		Dialer:         d.Dialer,
	//		EsClient:       esClient,
	//	},
	//)
	//if err != nil {
	//	return results.WithError(err)
	//}
	//if !done {
	//	log.V(1).Info("Pods restart is not over yet, re-queueing.")
	//	return results.WithResult(defaultRequeue)
	//}
	//
	//// figure out what changes we can perform right now
	//performableChanges, err := mutation.CalculatePerformableChanges(es.Spec.UpdateStrategy, *changes, podsState)
	//if err != nil {
	//	return results.WithError(err)
	//}
	//
	//log.Info(
	//	"Calculated performable changes",
	//	"schedule_for_creation_count", len(performableChanges.ToCreate),
	//	"schedule_for_deletion_count", len(performableChanges.ToDelete),
	//)

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

	// TODO: this is a mess, refactor and unit test correctly
	podTemplateSpecBuilder := func(nodeSpec v1alpha1.NodeSpec, cfg settings.CanonicalConfig) (corev1.PodTemplateSpec, error) {
		return esversion.BuildPodTemplateSpec(
			es,
			nodeSpec,
			pod.NewPodSpecParams{
				ProbeUser:    internalUsers.ProbeUser.Auth(),
				KeystoreUser: internalUsers.KeystoreUser.Auth(),
				UnicastHostsVolume: volume.NewConfigMapVolume(
					name.UnicastHostsConfigMap(es.Name), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
				),
				UsersSecretVolume: volume.NewSecretVolumeWithMountPath(
					user.XPackFileRealmSecretName(es.Name),
					esvolume.XPackFileRealmVolumeName,
					esvolume.XPackFileRealmVolumeMountPath,
				),
			},
			cfg,
			version6.NewEnvironmentVars,
			initcontainer.NewInitContainers,
			d.OperatorImage,
		)
	}

	if err := d.reconcileNodeSpecs(results, es, podTemplateSpecBuilder, esClient, observedState); err != nil {
		return results.WithError(err)
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
	//
	//// List the orphaned PVCs before the Pods are created.
	//// If there are some orphaned PVCs they will be adopted and remove sequentially from the list when Pods are created.
	//orphanedPVCs, err := pvc.FindOrphanedVolumeClaims(d.Client, es)
	//if err != nil {
	//	return results.WithError(err)
	//}
	//
	//for _, change := range performableChanges.ToCreate {
	//	d.PodsExpectations.ExpectCreation(namespacedName)
	//	if err := createElasticsearchPod(
	//		d.Client,
	//		d.Scheme,
	//		es,
	//		reconcileState,
	//		change.Pod,
	//		change.PodSpecCtx,
	//		orphanedPVCs,
	//	); err != nil {
	//		// pod was not created, cancel our expectation by marking it observed
	//		d.PodsExpectations.CreationObserved(namespacedName)
	//		return results.WithError(err)
	//	}
	//}
	// passed this point, any pods resource listing should check expectations first

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES external service not ready yet for shard migration reconciliation. Requeuing.")

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
	//
	//if !changes.HasChanges() {
	//	// Current state matches expected state
	//	reconcileState.UpdateElasticsearchOperational(*resourcesState, observedState)
	//	return results
	//}
	//
	//// Start migrating data away from all pods to be deleted
	//leavingNodeNames := pod.PodListToNames(performableChanges.ToDelete.Pods())
	//if err = migration.MigrateData(esClient, leavingNodeNames); err != nil {
	//	return results.WithError(errors.Wrap(err, "error during migrate data"))
	//}
	//
	//// Shrink clusters by deleting deprecated pods
	//if err = d.attemptPodsDeletion(
	//	performableChanges,
	//	reconcileState,
	//	resourcesState,
	//	observedState,
	//	results,
	//	esClient,
	//	es,
	//); err != nil {
	//	return results.WithError(err)
	//}
	//// past this point, any pods resource listing should check expectations first
	//
	//if changes.HasChanges() && !performableChanges.HasChanges() {
	//	// if there are changes we'd like to perform, but none that were performable, we try again later
	//	results.WithResult(defaultRequeue)
	//}

	reconcileState.UpdateElasticsearchState(*resourcesState, observedState)

	return results
}

//
//// attemptPodsDeletion deletes a list of pods after checking there is no migrating data for each of them
//func (d *defaultDriver) attemptPodsDeletion(
//	changes *mutation.PerformableChanges,
//	reconcileState *reconcile.State,
//	resourcesState *reconcile.ResourcesState,
//	observedState observer.State,
//	results *reconciler.Results,
//	esClient esclient.Client,
//	elasticsearch v1alpha1.Elasticsearch,
//) error {
//	newState := make([]corev1.Pod, len(resourcesState.CurrentPods))
//	copy(newState, resourcesState.CurrentPods.Pods())
//	for _, pod := range changes.ToDelete.Pods() {
//		newState = removePodFromList(newState, pod)
//		preDelete := func() error {
//			if d.zen1SettingsUpdater != nil {
//				requeue, err := d.zen1SettingsUpdater(
//					elasticsearch,
//					d.Client,
//					esClient,
//					newState,
//					changes,
//					reconcileState)
//
//				if err != nil {
//					return err
//				}
//
//				if requeue {
//					results.WithResult(defaultRequeue)
//				}
//			}
//			return nil
//		}
//
//		// do not delete a pod or expect a deletion if a data migration is in progress
//		isMigratingData := migration.IsMigratingData(observedState, pod, changes.ToDelete.Pods())
//		if isMigratingData {
//			log.Info("Skipping deletion because of migrating data", "pod", pod.Name)
//			reconcileState.UpdateElasticsearchMigrating(*resourcesState, observedState)
//			results.WithResult(defaultRequeue)
//			continue
//		}
//
//		namespacedName := k8s.ExtractNamespacedName(&elasticsearch)
//		d.PodsExpectations.ExpectDeletion(namespacedName)
//		result, err := deleteElasticsearchPod(
//			d.Client,
//			reconcileState,
//			*resourcesState,
//			pod,
//			preDelete,
//		)
//		if err != nil {
//			// pod was not deleted, cancel our expectation by marking it observed
//			d.PodsExpectations.DeletionObserved(namespacedName)
//			return err
//		}
//		results.WithResult(result)
//	}
//	return nil
//}

// removePodFromList removes a single pod from the list, matching by pod name.
func removePodFromList(pods []corev1.Pod, pod corev1.Pod) []corev1.Pod {
	for i, p := range pods {
		if p.Name == pod.Name {
			return append(pods[:i], pods[i+1:]...)
		}
	}
	return pods
}

func (d *defaultDriver) reconcileNodeSpecs(
	results *reconciler.Results,
	es v1alpha1.Elasticsearch,
	podSpecBuilder esversion.PodTemplateSpecBuilder,
	esClient esclient.Client,
	observedState observer.State,
) error {

	actualStatefulSets, err := sset.RetrieveActualStatefulSets(d.Client, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return err
	}

	nodeSpecResources, err := nodespec.BuildExpectedResources(es, podSpecBuilder)
	if err != nil {
		return err
	}

	// TODO: handle zen2 initial master nodes more cleanly
	//  should be empty once cluster is bootstraped
	var initialMasters []string
	// TODO: refactor/move
	for _, res := range nodeSpecResources {
		cfg, err := res.Config.Unpack()
		if err != nil {
			return err
		}
		if cfg.Node.Master {
			for i := 0; i < int(*res.StatefulSet.Spec.Replicas); i++ {
				initialMasters = append(initialMasters, fmt.Sprintf("%s-%d", res.StatefulSet.Name, i))
			}
		}
	}
	for i := range nodeSpecResources {
		if err := nodeSpecResources[i].Config.SetStrings(settings.ClusterInitialMasterNodes, initialMasters...); err != nil {
			return err
		}
	}

	// Phase 1: apply expected StatefulSets resources, but don't scale down.
	// The goal is to:
	// 1. scale sset up (eg. go from 3 to 5 replicas).
	// 2. apply configuration changes on the sset resource, to be used for future pods creation/recreation,
	//    but do not rotate pods yet.
	// 3. do **not** apply replicas scale down, otherwise nodes would be deleted before
	//    we handle a clean deletion.
	for _, nodeSpec := range nodeSpecResources {
		// always reconcile config (will apply to new & recreated pods)
		if err := settings.ReconcileConfig(d.Client, es, nodeSpec.StatefulSet.Name, nodeSpec.Config); err != nil {
			return err
		}
		if _, err := common.ReconcileService(d.Client, d.Scheme, &nodeSpec.HeadlessService, &es); err != nil {
			return err
		}
		ssetToApply := nodeSpec.StatefulSet.DeepCopy()
		actual, exists := actualStatefulSets.GetByName(ssetToApply.Name)
		if exists && *ssetToApply.Spec.Replicas < *actual.Spec.Replicas {
			// sset needs to be scaled down
			// update the sset to use the new spec but don't scale replicas down for now
			ssetToApply.Spec.Replicas = actual.Spec.Replicas
		}
		if err := sset.ReconcileStatefulSet(d.Client, d.Scheme, es, *ssetToApply); err != nil {
			return err
		}
	}

	// Phase 2: handle sset scale down.
	// We want to safely remove nodes from the cluster, either because the sset requires less replicas,
	// or because it should be removed entirely.
	for i, actual := range actualStatefulSets {
		expected, shouldExist := nodeSpecResources.StatefulSets().GetByName(actual.Name)
		switch {
		// stateful set removal
		case !shouldExist:
			target := 0
			if err := d.scaleStatefulSetDown(results, &actualStatefulSets[i], target, esClient, observedState); err != nil {
				return err
			}
		// stateful set downscale
		case actual.Spec.Replicas != nil && *expected.Spec.Replicas < *actual.Spec.Replicas:
			target := int(*expected.Spec.Replicas)
			if err := d.scaleStatefulSetDown(results, &actualStatefulSets[i], target, esClient, observedState); err != nil {
				return err
			}
		}
	}

	// TODO:
	//  - safe node upgrade (rollingUpdate.Partition + shards allocation)
	//  - change budget
	//  - zen1, zen2
	return nil
}

func (d *defaultDriver) scaleStatefulSetDown(
	results *reconciler.Results,
	statefulSet *appsv1.StatefulSet,
	targetReplicas int,
	esClient esclient.Client,
	observedState observer.State,
) error {
	logger := log.WithValues("statefulset", k8s.ExtractNamespacedName(statefulSet))

	if *statefulSet.Spec.Replicas == 0 && targetReplicas == 0 {
		// we don't expect any new replicas in this statefulset, remove it
		logger.Info("Deleting statefulset")
		if err := d.Client.Delete(statefulSet); err != nil {
			return err
		}
	}
	// copy the current replicas, to be decremented with nodes to remove
	updatedReplicas := int32(*statefulSet.Spec.Replicas)

	// leaving nodes names can be built from StatefulSet name and ordinals
	var leavingNodes []string
	for i := int(*statefulSet.Spec.Replicas) - 1; i > targetReplicas-1; i-- {
		leavingNodes = append(leavingNodes, sset.PodName(statefulSet.Name, i))
	}

	// TODO: don't remove last master/last data nodes?
	// TODO: detect cases where data migration cannot happen since no nodes to host shards?

	// migrate data away from these nodes before removing them
	logger.V(1).Info("Migrating data away from nodes", "nodes", leavingNodes)
	if err := migration.MigrateData(esClient, leavingNodes); err != nil {
		return err
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

	if updatedReplicas != *statefulSet.Spec.Replicas {
		// update cluster coordination settings to account for nodes deletion
		// TODO: update zen1/zen2

		// trigger deletion of nodes whose data migration is over
		logger.V(1).Info("Scaling replicas down", "from", *statefulSet.Spec.Replicas, "to", updatedReplicas)
		statefulSet.Spec.Replicas = &updatedReplicas
		if err := d.Client.Update(statefulSet); err != nil {
			return err
		}
	}

	return nil
}

//
//// calculateChanges calculates the changes we'd need to perform to go from the current cluster configuration to the
//// desired one.
//func (d *defaultDriver) calculateChanges(
//	internalUsers *user.InternalUsers,
//	es v1alpha1.Elasticsearch,
//	resourcesState reconcile.ResourcesState,
//) (*mutation.Changes, error) {
//	expectedPodSpecCtxs, err := d.expectedPodsAndResourcesResolver(
//		es,
//		pod.NewPodSpecParams{
//			ProbeUser:    internalUsers.ProbeUser.Auth(),
//			KeystoreUser: internalUsers.KeystoreUser.Auth(),
//			UnicastHostsVolume: volume.NewConfigMapVolume(
//				name.UnicastHostsConfigMap(es.Name), esvolume.UnicastHostsVolumeName, esvolume.UnicastHostsVolumeMountPath,
//			),
//		},
//		d.OperatorImage,
//	)
//	if err != nil {
//		return nil, err
//	}
//
//	changes, err := mutation.CalculateChanges(
//		es,
//		expectedPodSpecCtxs,
//		resourcesState,
//		func(ctx pod.PodSpecContext) corev1.Pod {
//			return esversion.NewPod(es, ctx)
//		},
//	)
//	if err != nil {
//		return nil, err
//	}
//	return &changes, nil
//}

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
