// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pvc"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type failureTestFunc func(k *helpers.K8sHelper) helpers.TestStepList

func RunFailureTest(t *testing.T, s stack.Builder, f failureTestFunc) {
	k := helpers.NewK8sClientOrFatal()

	var clusterUUID string

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(stack.RetrieveClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		// Trigger some kind of catastrophe
		WithSteps(f(k)...).
		// Check we recover
		WithSteps(stack.CheckStackSteps(s, k)...).
		// And that the cluster UUID has not changed
		WithSteps(stack.CompareClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		WithSteps(stack.DeletionTestSteps(s, k)...).
		RunSequential(t)
}

func killNodeTest(t *testing.T, s stack.Builder, listOptions client.ListOptions, podMatch func(p corev1.Pod) bool) {
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		var killedPod corev1.Pod
		return helpers.TestStepList{
			{
				Name: "Kill a node",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(listOptions)
					require.NoError(t, err)
					var found bool
					killedPod, found = helpers.GetFirstPodMatching(pods, podMatch)
					require.True(t, found)
					err = k.DeletePod(killedPod)
					require.NoError(t, err)
				},
			},
			{
				Name: "Wait for pod to be deleted",
				Test: helpers.Eventually(func() error {
					pod, err := k.GetPod(killedPod.Name)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					if apierrors.IsNotFound(err) || killedPod.UID != pod.UID {
						return nil
					}
					return fmt.Errorf("pod %s not deleted yet", killedPod.Name)
				}),
			},
		}
	})
}

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	s := stack.NewStackBuilder("test-failure-kill-one-data-node").
		WithESMasterNodes(1, stack.DefaultResources).
		WithESDataNodes(2, stack.DefaultResources)
	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Elasticsearch.Name), matchDataNode)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	s := stack.NewStackBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, stack.DefaultResources).
		WithESDataNodes(2, stack.DefaultResources)
	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Elasticsearch.Name), matchMasterNode)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-pvc").
		WithESMasterDataNodes(1, stack.DefaultResources)
	matchNode := func(p corev1.Pod) bool {
		return true // match first node we find
	}
	killNodeTest(t, s, helpers.ESPodListOptions(s.Elasticsearch.Name), matchNode)
}

// TestKillCorrectPVReuse sets up a cluster with multiple PVs per node, kills a node, then makes sure that:
// - PVC are reused with the correct volume (eg. do not bind the "data" PVC to a "non-data" volume)
// - if no PVC is available, a new one is created
func TestKillCorrectPVReuse(t *testing.T) {
	helpers.MinVersionOrSkip(t, "v1.12.0")

	k := helpers.NewK8sClientOrFatal()

	// When working with multiple PVs on a single pod, there's a risk each PV get assigned to a different zone,
	// not taking into consideration pod scheduling constraints. As a result, the pod becomes unschedulable.
	// This is a Kubernetes issue, that can be dealt with relying on storage classes with `volumeBindingMode: WaitForFirstConsumer`.
	// With this binding mode, the pod is scheduled before its PVs, which then take into account zone constraints.
	// That's the only way to work with multiple PVs. Since the k8s cluster here may have a default storage class
	// with `volumeBindingMode: Immediate`, we create a new one, based on the default storage class, that uses
	// `waitForFirstConsumer`.
	lateBinding := v1.VolumeBindingWaitForFirstConsumer
	sc, err := stack.DefaultStorageClass(k)
	require.NoError(t, err)
	sc.ObjectMeta = metav1.ObjectMeta{
		Name: "custom-storage",
	}
	sc.VolumeBindingMode = &lateBinding

	s := stack.NewStackBuilder("test-failure-pvc").
		WithESMasterDataNodes(3, stack.DefaultResources).
		WithPersistentVolumes("not-data", &sc.Name).
		WithPersistentVolumes(volume.ElasticsearchDataVolumeName, &sc.Name) // create an additional volume that is not our data volume

	var clusterUUID string
	var deletedPVC corev1.PersistentVolumeClaim
	var seenPVCs []string
	var killedPod corev1.Pod

	helpers.TestStepList{}.
		WithSteps(stack.CreateStorageClass(*sc, k)).
		WithSteps(stack.InitTestSteps(s, k)...).
		WithSteps(stack.CreationTestSteps(s, k)...).
		WithSteps(stack.RetrieveClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		// Simulate a pod deletion
		WithSteps(stack.PauseReconciliation(s.Elasticsearch, k)).
		WithSteps(
			helpers.TestStep{
				Name: "Kill a node",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(helpers.ESPodListOptions(s.Elasticsearch.Name))
					require.NoError(t, err)
					require.True(t, len(pods) > 0, "need at least one pod to kill")
					for i, pod := range pods {
						if i == 0 {
							killedPod = pod
						}
					}
					err = k.DeletePod(killedPod)
					require.NoError(t, err)
				},
			},
			helpers.TestStep{
				Name: "Wait for pod to be deleted",
				Test: helpers.Eventually(func() error {
					pod, err := k.GetPod(killedPod.Name)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					if apierrors.IsNotFound(err) || killedPod.UID != pod.UID {
						return nil
					}
					return fmt.Errorf("pod %s not deleted yet", killedPod.Name)
				}),
			},
			helpers.TestStep{
				Name: "Delete one of the es-data PVCs",
				Test: helpers.Eventually(func() error {
					pvcs, err := pvc.ListVolumeClaims(k.Client, s.Elasticsearch)
					if err != nil {
						return err
					}
					for _, pvc := range pvcs {
						seenPVCs = append(seenPVCs, string(pvc.UID))
						if pvc.Labels[label.VolumeNameLabelName] == volume.ElasticsearchDataVolumeName &&
							pvc.Labels[label.PodNameLabelName] == killedPod.Name {
							// this should ensure that when we resume reconciliation the operator creates a new PVC
							// we also test correct reuse by keeping the non-data volume claim around and unchanged
							deletedPVC = pvc
							if err := k.Client.Delete(&pvc); err != nil {
								return err
							}
						}
					}
					return nil
				}),
			},
			stack.ResumeReconciliation(s.Elasticsearch, k)).
		// Check we recover
		WithSteps(stack.CheckStackSteps(s, k)...).
		// Check PVCs have been reused correctly
		WithSteps(
			helpers.TestStep{
				Name: "No PVC should have been reused for elasticsearch-data",
				Test: func(t *testing.T) {
					// should be resurrected with same name due to second PVC still around and forcing the pods name
					// back to the old one
					pod, err := k.GetPod(killedPod.Name)
					require.NoError(t, err)
					var checkedVolumes bool
					for _, v := range pod.Spec.Volumes {
						// find the volumes sourced from PVCs
						pvcSrc := v.VolumeSource.PersistentVolumeClaim
						if pvcSrc == nil {
							// we have a few non-PVC volumes
							continue
						}
						checkedVolumes = true
						// fetch the corresponding claim
						var pvc corev1.PersistentVolumeClaim
						require.NoError(t, k.Client.Get(types.NamespacedName{Namespace: pod.Namespace, Name: pvcSrc.ClaimName}, &pvc))

						// for elasticsearch-data ensure it's a new one (we deleted the old one above)
						if v.Name == volume.ElasticsearchDataVolumeName && deletedPVC.UID == pvc.UID {
							t.Errorf("expected new PVC but was reused %v, %v, seen: %v", pvc.Name, pvc.UID, deletedPVC.UID)
							// for all the other volumes expect reuse
						} else if v.Name != volume.ElasticsearchDataVolumeName && !stringsutil.StringInSlice(string(pvc.UID), seenPVCs) {
							t.Errorf("expected reused PVC but %v is new, %v , seen: %v", pvc.Name, pvc.UID, seenPVCs)
						}
					}
					require.True(t, checkedVolumes, "unexpected: no persistent volume claims where found")
				},
			},
		).
		// And that the cluster UUID has not changed
		WithSteps(stack.CompareClusterUUIDStep(s.Elasticsearch, k, &clusterUUID)).
		WithSteps(stack.DeletionTestSteps(s, k)...).
		RunSequential(t)
}

func TestKillKibanaPod(t *testing.T) {
	s := stack.NewStackBuilder("test-kill-kibana-pod").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1)
	matchFirst := func(p corev1.Pod) bool {
		return true
	}
	killNodeTest(t, s, helpers.KibanaPodListOptions(s.Kibana.Name), matchFirst)
}

func TestKillKibanaDeployment(t *testing.T) {
	s := stack.NewStackBuilder("test-kill-kibana-deployment").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete Kibana deployment",
				Test: func(t *testing.T) {
					var dep appsv1.Deployment
					err := k.Client.Get(types.NamespacedName{
						Namespace: params.Namespace,
						Name:      kbname.Deployment(s.Kibana.Name),
					}, &dep)
					require.NoError(t, err)
					err = k.Client.Delete(&dep)
					require.NoError(t, err)
				},
			},
		}
	})
}

func TestDeleteServices(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, stack.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete external service",
				Test: func(t *testing.T) {
					s, err := k.GetService(name.HTTPService(s.Elasticsearch.Name))
					require.NoError(t, err)
					err = k.Client.Delete(s)
					require.NoError(t, err)
				},
			},
		}
	})
}

func TestDeleteElasticUserSecret(t *testing.T) {
	s := stack.NewStackBuilder("test-delete-es-elastic-user-secret").
		WithESMasterDataNodes(1, stack.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: params.Namespace,
						Name:      s.Elasticsearch.Name + "-es-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	})
}
func TestDeleteCACert(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, stack.DefaultResources)
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		return helpers.TestStepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: params.Namespace,
						Name:      s.Elasticsearch.Name + "-es-transport-ca-internal", // ~that's the CA cert secret name \o/~ ... oops not anymore
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	})
}
