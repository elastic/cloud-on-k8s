// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pvc"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestKillOneDataNode(t *testing.T) {
	// 1 master + 2 data nodes
	b := elasticsearch.NewBuilder("test-failure-kill-one-data-node").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)

	matchDataNode := func(p corev1.Pod) bool {
		return label.IsDataNode(p) && !label.IsMasterNode(p)
	}

	test.RunFailure(t,
		test.KillNodeSteps(test.ESPodListOptions(b.Elasticsearch.Name), matchDataNode),
		b)
}

func TestKillOneMasterNode(t *testing.T) {
	// 2 master + 2 data nodes
	b := elasticsearch.NewBuilder("test-failure-kill-one-master-node").
		WithESMasterNodes(2, elasticsearch.DefaultResources).
		WithESDataNodes(2, elasticsearch.DefaultResources)

	matchMasterNode := func(p corev1.Pod) bool {
		return !label.IsDataNode(p) && label.IsMasterNode(p)
	}

	test.RunFailure(t,
		test.KillNodeSteps(test.ESPodListOptions(b.Elasticsearch.Name), matchMasterNode),
		b)
}

func TestKillSingleNodeReusePV(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-pvc").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	matchNode := func(p corev1.Pod) bool {
		return true // match first node we find
	}

	test.RunFailure(t,
		test.KillNodeSteps(test.ESPodListOptions(b.Elasticsearch.Name), matchNode),
		b)
}

// TestKillCorrectPVReuse sets up a cluster with multiple PVs per node, kills a node, then makes sure that:
// - PVC are reused with the correct volume (eg. do not bind the "data" PVC to a "non-data" volume)
// - if no PVC is available, a new one is created
func TestKillCorrectPVReuse(t *testing.T) {
	test.MinVersionOrSkip(t, "v1.12.0")

	k := test.NewK8sClientOrFatal()

	// When working with multiple PVs on a single pod, there's a risk each PV get assigned to a different zone,
	// not taking into consideration pod scheduling constraints. As a result, the pod becomes unschedulable.
	// This is a Kubernetes issue, that can be dealt with relying on storage classes with `volumeBindingMode: WaitForFirstConsumer`.
	// With this binding mode, the pod is scheduled before its PVs, which then take into account zone constraints.
	// That's the only way to work with multiple PVs. Since the k8s cluster here may have a default storage class
	// with `volumeBindingMode: Immediate`, we create a new one, based on the default storage class, that uses
	// `waitForFirstConsumer`.
	lateBinding := v1.VolumeBindingWaitForFirstConsumer
	sc, err := elasticsearch.DefaultStorageClass(k)
	require.NoError(t, err)
	sc.ObjectMeta = metav1.ObjectMeta{
		Name: "custom-storage",
	}
	sc.VolumeBindingMode = &lateBinding

	b := elasticsearch.NewBuilder("test-failure-pvc").
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithPersistentVolumes("not-data", &sc.Name).
		WithPersistentVolumes(volume.ElasticsearchDataVolumeName, &sc.Name) // create an additional volume that is not our data volume

	var clusterUUID string
	var deletedPVC corev1.PersistentVolumeClaim
	var seenPVCs []string
	var killedPod corev1.Pod

	test.StepList{}.
		WithStep(elasticsearch.CreateStorageClass(*sc, k)).
		WithSteps(b.InitTestSteps(k)).
		WithSteps(b.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(b, k)).
		WithStep(elasticsearch.RetrieveClusterUUIDStep(b.Elasticsearch, k, &clusterUUID)).
		// Simulate a pod deletion
		WithStep(elasticsearch.PauseReconciliation(b.Elasticsearch, k)).
		WithSteps(test.StepList{
			{
				Name: "Kill a node",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(test.ESPodListOptions(b.Elasticsearch.Name))
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
			{
				Name: "Wait for pod to be deleted",
				Test: test.Eventually(func() error {
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
			{
				Name: "Delete one of the es-data PVCs",
				Test: test.Eventually(func() error {
					pvcs, err := pvc.ListVolumeClaims(k.Client, b.Elasticsearch)
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
			elasticsearch.ResumeReconciliation(b.Elasticsearch, k),
		}).
		// Check we recover
		WithSteps(test.CheckTestSteps(b, k)).
		// Check PVCs have been reused correctly
		WithStep(test.Step{
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
		WithStep(elasticsearch.CompareClusterUUIDStep(b.Elasticsearch, k, &clusterUUID)).
		WithSteps(b.DeletionTestSteps(k)).
		RunSequential(t)
}

func TestDeleteServices(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-delete-services").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.RunFailure(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete external service",
				Test: func(t *testing.T) {
					s, err := k.GetService(esname.HTTPService(b.Elasticsearch.Name))
					require.NoError(t, err)
					err = k.Client.Delete(s)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}

func TestDeleteElasticUserSecret(t *testing.T) {
	b := elasticsearch.NewBuilder("test-delete-es-elastic-user-secret").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.RunFailure(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete elastic user secret",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: test.Namespace,
						Name:      b.Elasticsearch.Name + "-es-elastic-user",
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}
func TestDeleteCACert(t *testing.T) {
	b := elasticsearch.NewBuilder("test-failure-delete-ca-cert").
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	test.RunFailure(t, func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Delete CA cert",
				Test: func(t *testing.T) {
					key := types.NamespacedName{
						Namespace: test.Namespace,
						Name:      b.Elasticsearch.Name + "-es-transport-ca-internal", // ~that's the CA cert secret name \o/~ ... oops not anymore
					}
					var secret corev1.Secret
					err := k.Client.Get(key, &secret)
					require.NoError(t, err)
					err = k.Client.Delete(&secret)
					require.NoError(t, err)
				},
			},
		}
	}, b)
}
