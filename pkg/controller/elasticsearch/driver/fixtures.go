// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type testPod struct {
	name                                        string
	master, data, healthy, toUpgrade, inCluster bool
	uid                                         types.UID
}

func newTestPod(name string) testPod {
	return testPod{
		name: name,
		uid:  uuid.NewUUID(),
	}
}

func (t testPod) isMaster(v bool) testPod     { t.master = v; return t }
func (t testPod) isData(v bool) testPod       { t.data = v; return t }
func (t testPod) isInCluster(v bool) testPod  { t.inCluster = v; return t }
func (t testPod) isHealthy(v bool) testPod    { t.healthy = v; return t }
func (t testPod) needsUpgrade(v bool) testPod { t.toUpgrade = v; return t }

// filter to simulate a Pod tha has been removed while upgrading
// unfortunately fake client does not support predicate
type filter func(pod corev1.Pod) bool

var nothing = func(pod corev1.Pod) bool {
	return false
}

func byName(name string) filter {
	return func(pod corev1.Pod) bool {
		return pod.Name == name
	}
}

type upgradeTestPods []testPod

func newUpgradeTestPods(pods ...testPod) upgradeTestPods {
	result := make(upgradeTestPods, len(pods))
	for i := range pods {
		result[i] = pods[i]
	}
	return result
}

func (u upgradeTestPods) toES(maxUnavailable int) v1alpha1.Elasticsearch {
	return v1alpha1.Elasticsearch{
		Spec: v1alpha1.ElasticsearchSpec{
			UpdateStrategy: v1alpha1.UpdateStrategy{
				ChangeBudget: &v1alpha1.ChangeBudget{
					MaxUnavailable: maxUnavailable,
				},
			},
		},
	}
}

// Infer the list of statefulsets from the Pods used in the test
func (u upgradeTestPods) toStatefulSetList() sset.StatefulSetList {
	// Get all the statefulsets
	statefulSets := make(map[string]int32)
	for _, testPod := range u {
		name, ordinal, err := sset.StatefulSetName(testPod.name)
		if err != nil {
			panic(err)
		}
		if replicas, found := statefulSets[name]; found {
			if ordinal > replicas {
				statefulSets[name] = ordinal
			}
		} else {
			statefulSets[name] = ordinal
		}
	}
	statefulSetList := make(sset.StatefulSetList, len(statefulSets))
	i := 0
	for statefulSet, replica := range statefulSets {
		statefulSetList[i] = sset.TestSset{Name: statefulSet, Replicas: replica + 1}.Build()
		i++
	}
	return statefulSetList
}

func (u upgradeTestPods) toPods(f filter) []runtime.Object {
	var result []runtime.Object
	i := 0
	for _, testPod := range u {
		pod := testPod.toPod()
		if !f(pod) {
			result = append(result, &pod)
		}
		i++
	}
	return result
}

func (u upgradeTestPods) toHealthyPods() map[string]corev1.Pod {
	result := make(map[string]corev1.Pod)
	for _, testPod := range u {
		pod := testPod.toPod()
		if k8s.IsPodReady(pod) && testPod.inCluster {
			result[pod.Name] = pod
		}
	}
	return result
}

func (u upgradeTestPods) toUpgrade() []corev1.Pod {
	var result []corev1.Pod
	for _, testPod := range u {
		pod := testPod.toPod()
		if testPod.toUpgrade {
			result = append(result, pod)
		}
	}
	return result
}

func (u upgradeTestPods) podsInCluster() []string {
	var result []string
	for _, testPod := range u {
		pod := testPod.toPod()
		if testPod.inCluster {
			result = append(result, pod.Name)
		}
	}
	return result
}

func (u upgradeTestPods) toMasters() []string {
	var result []string
	for _, testPod := range u {
		pod := testPod.toPod()
		if label.IsMasterNode(pod) {
			result = append(result, pod.Name)
		}
	}
	return result
}

func names(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for i, pod := range pods {
		result[i] = pod.Name
	}
	return result
}

func (t testPod) toPod() corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.name,
			Namespace: "testNS",
			UID:       t.uid,
		},
	}
	labels := map[string]string{}
	label.NodeTypesMasterLabelName.Set(t.master, labels)
	label.NodeTypesDataLabelName.Set(t.data, labels)
	pod.Labels = labels
	if t.healthy {
		pod.Status = corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.ContainersReady,
					Status: corev1.ConditionTrue,
				},
			},
		}
	}
	return pod
}

type testESState struct {
	inCluster []string
	green     bool
	ESState
}

func (t *testESState) ShardAllocationsEnabled() (bool, error) {
	return true, nil
}

func (t *testESState) GreenHealth() (bool, error) {
	return t.green, nil
}

func (t *testESState) NodesInCluster(nodeNames []string) (bool, error) {
	for _, nodeName := range nodeNames {
		for _, inClusterPods := range t.inCluster {
			if nodeName == inClusterPods {
				return true, nil
			}
		}
	}
	return false, nil
}

func loadFileBytes(fileName string) []byte {
	contents, err := ioutil.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		panic(err)
	}

	return contents
}

func (t *testESState) GetClusterState() (*esclient.ClusterState, error) {
	var cs esclient.ClusterState
	sampleClusterState := loadFileBytes("cluster_state.json")
	err := json.Unmarshal(sampleClusterState, &cs)
	return &cs, err
}
