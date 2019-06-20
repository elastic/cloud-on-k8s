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
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/kubelet/util/sliceutils"
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

func killSteps(listOptions client.ListOptions, podMatch func(p corev1.Pod) bool) failureTestFunc {
	var killedPod corev1.Pod
	return func(k *helpers.K8sHelper) helpers.TestStepList {
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
					return fmt.Errorf("Pod %s not deleted yet", killedPod.Name)
				}),
			},
		}
	}
}

func killNodeTest(t *testing.T, s stack.Builder, listOptions client.ListOptions, podMatch func(p corev1.Pod) bool) {
	RunFailureTest(t, s, killSteps(listOptions, podMatch))
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

func TestKillCorrectPVReuse(t *testing.T) {
	s := stack.NewStackBuilder("test-failure-pvc").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithAdditionalPersistentVolumes() // create an additional volume that is not our data volume

	matchNode := func(p corev1.Pod) bool {
		return true // match first node we find
	}
	RunFailureTest(t, s, func(k *helpers.K8sHelper) helpers.TestStepList {
		var seenPVCs []string
		list := helpers.TestStepList{}
		list = append(list, stack.PauseReconciliation(s.Elasticsearch, k))
		list = append(list, killSteps(helpers.ESPodListOptions(s.Elasticsearch.Name), matchNode)(k)...)
		list = append(list, helpers.TestStep{
			Name: "Modify the es-data PVCs labels",
			Test: helpers.Eventually(func() error {
				pvcs, err := pvc.ListVolumeClaims(k.Client, s.Elasticsearch)
				if err != nil {
					return err
				}
				for _, pvc := range pvcs {
					seenPVCs = append(seenPVCs, pvc.Name)
					if pvc.Labels[label.VolumeNameLabelName] == volume.ElasticsearchDataVolumeName {
						// this should ensure that when we resume reconciliation the operator creates a new PVC
						// we also test correct reuse by keeping the non-data volume claim around and unchanged
						delete(pvc.Labels, label.VolumeNameLabelName)
						if err := k.Client.Update(&pvc); err != nil {
							return err
						}
					}
				}
				return nil
			}),
		})
		list = append(list, stack.ResumeReconciliation(s.Elasticsearch, k))
		list = append(list, helpers.TestStep{
			Name: "No PVC should have been reused for elasticsearch-data",
			Test: helpers.Eventually(func() error {
				pods, err := k.GetPods(helpers.ESPodListOptions(s.Elasticsearch.Name))
				if err != nil {
					return err
				}
				for _, pod := range pods {
					for _, v := range pod.Spec.Volumes {
						pvc := v.VolumeSource.PersistentVolumeClaim
						if pvc == nil {
							continue
						}
						if v.Name != volume.ElasticsearchDataVolumeName {
							if !sliceutils.StringInSlice(pvc.ClaimName, seenPVCs) {
								return fmt.Errorf("expected reused PVC but %v is new , seen: %v", pvc.ClaimName, seenPVCs)
							}

						} else if sliceutils.StringInSlice(pvc.ClaimName, seenPVCs) {
							return fmt.Errorf("expected new PVC but was reused %v, seen: %v", pvc.ClaimName, seenPVCs)
						}
					}
				}
				return nil
			}),
		})
		return list
	})
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
