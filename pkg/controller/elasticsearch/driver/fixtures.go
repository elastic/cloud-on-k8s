// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const (
	TestEsName      = "TestES"
	TestEsNamespace = "TestNS"
)

type testPod struct {
	name                                                     string
	version                                                  string
	ssetName                                                 string
	master, data, healthy, toUpgrade, inCluster, terminating bool
	uid                                                      types.UID
	resourceVersion                                          string
}

func newTestPod(name string) testPod {
	return testPod{
		name: name,
		uid:  uuid.NewUUID(),
	}
}

func (t testPod) isMaster(v bool) testPod               { t.master = v; return t }
func (t testPod) isData(v bool) testPod                 { t.data = v; return t }
func (t testPod) isInCluster(v bool) testPod            { t.inCluster = v; return t }
func (t testPod) isHealthy(v bool) testPod              { t.healthy = v; return t }
func (t testPod) needsUpgrade(v bool) testPod           { t.toUpgrade = v; return t }
func (t testPod) isTerminating(v bool) testPod          { t.terminating = v; return t }
func (t testPod) withVersion(v string) testPod          { t.version = v; return t }
func (t testPod) inStatefulset(ssetName string) testPod { t.ssetName = ssetName; return t }
func (t testPod) withResourceVersion(rv string) testPod { t.resourceVersion = rv; return t } //nolint:unparam

// filter to simulate a Pod that has been removed while upgrading
// unfortunately fake client does not support predicate
type filter func(pod corev1.Pod) bool

// -- Filters

var nothing = func(pod corev1.Pod) bool {
	return false
}

func byName(name string) filter {
	return func(pod corev1.Pod) bool {
		return pod.Name == name
	}
}

// - Mutations are used to simulate a type change on a set of Pods, e.g. MD -> D or D -> MD

type mutation func(pod corev1.Pod) corev1.Pod

var noMutation = func(pod corev1.Pod) corev1.Pod {
	return pod
}

func removeMasterType(ssetName string) mutation {
	return func(pod corev1.Pod) corev1.Pod {
		podSsetname, _, _ := sset.StatefulSetName(pod.Name)
		if podSsetname == ssetName {
			pod := pod.DeepCopy()
			label.NodeTypesMasterLabelName.Set(false, pod.Labels)
			return *pod
		}
		return pod
	}
}

func addMasterType(ssetName string) mutation {
	return func(pod corev1.Pod) corev1.Pod {
		podSsetname, _, _ := sset.StatefulSetName(pod.Name)
		if podSsetname == ssetName {
			pod := pod.DeepCopy()
			label.NodeTypesMasterLabelName.Set(true, pod.Labels)
			return *pod
		}
		return pod
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

func (u upgradeTestPods) toES(version string, maxUnavailable int) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestEsName,
			Namespace: TestEsNamespace,
		},
		Spec: esv1.ElasticsearchSpec{
			Version: version,
			UpdateStrategy: esv1.UpdateStrategy{
				ChangeBudget: esv1.ChangeBudget{
					MaxUnavailable: pointer.Int32(int32(maxUnavailable)),
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
		statefulSetList[i] = sset.TestSset{Name: statefulSet, ClusterName: TestEsName, Namespace: TestEsNamespace, Replicas: replica + 1}.Build()
		i++
	}
	return statefulSetList
}

func (u upgradeTestPods) toRuntimeObjects(version string, maxUnavailable int, f filter) []runtime.Object {
	var result []runtime.Object
	i := 0
	for _, testPod := range u {
		pod := testPod.toPod()
		if !f(pod) {
			result = append(result, &pod)
		}
		i++
	}
	es := u.toES(version, maxUnavailable)
	result = append(result, &es)
	return result
}

func (u upgradeTestPods) toMasterPods() []corev1.Pod {
	var result []corev1.Pod
	for _, testPod := range u {
		pod := testPod.toPod()
		if label.IsMasterNode(pod) {
			result = append(result, pod)
		}
	}
	return result
}

func (u upgradeTestPods) toHealthyPods() map[string]corev1.Pod {
	result := make(map[string]corev1.Pod)
	for _, testPod := range u {
		pod := testPod.toPod()
		if pod.DeletionTimestamp.IsZero() && k8s.IsPodReady(pod) && testPod.inCluster {
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

func (u upgradeTestPods) toMasters(mutation mutation) []string {
	var result []string
	for _, testPod := range u {
		pod := mutation(testPod.toPod())
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
	var deletionTimestamp *metav1.Time
	if t.terminating {
		now := metav1.Now()
		deletionTimestamp = &now
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              t.name,
			Namespace:         TestEsNamespace,
			UID:               t.uid,
			DeletionTimestamp: deletionTimestamp,
			ResourceVersion:   t.resourceVersion,
		},
	}
	labels := map[string]string{}
	labels[label.VersionLabelName] = t.version
	labels[label.ClusterNameLabelName] = TestEsName
	label.NodeTypesMasterLabelName.Set(t.master, labels)
	label.NodeTypesDataLabelName.Set(t.data, labels)
	labels[label.StatefulSetNameLabelName] = t.ssetName
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

func (t testPod) toPodPtr() *corev1.Pod {
	pod := t.toPod()
	return &pod
}

type testESState struct {
	inCluster []string
	health    client.Health
	ESState
}

func (t *testESState) ShardAllocationsEnabled() (bool, error) {
	return true, nil
}

func (t *testESState) Health() (client.Health, error) {
	return t.health, nil
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
