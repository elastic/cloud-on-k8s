// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

type RestartPhase string

// Restart phases annotated on pods
const (
	RestartPhaseAnnotation = "elasticsearch.k8s.elastic.co/restart-phase"

	PhaseSchedule RestartPhase = "schedule"
	PhaseStop     RestartPhase = "stop"
	PhaseStart    RestartPhase = "start"
)

func getPhase(pod corev1.Pod) (RestartPhase, bool) {
	phase, isSet := pod.Annotations[RestartPhaseAnnotation]
	return RestartPhase(phase), isSet
}

func hasPhase(pod corev1.Pod, expected RestartPhase) bool {
	actual, isSet := getPhase(pod)
	return isSet && actual == expected
}

func isAnnotatedForRestart(pod corev1.Pod) bool {
	_, annotated := getPhase(pod)
	return annotated
}

func setPhase(client k8s.Client, pod corev1.Pod, phase RestartPhase) error {
	log.V(1).Info(
		"Setting restart phase",
		"pod", pod.Name,
		"phase", phase,
	)
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[RestartPhaseAnnotation] = string(phase)
	return client.Update(&pod)
}

type RestartStrategy string

// Restart strategies annotated on pods
const (
	RestartStrategyAnnotation = "elasticsearch.k8s.elastic.co/restart-strategy"

	StrategySingle      RestartStrategy = "single"
	StrategyCoordinated RestartStrategy = "coordinated"
	StrategyRolling     RestartStrategy = "rolling"
)

func getStrategy(pod corev1.Pod) RestartStrategy {
	strategy, isSet := pod.Annotations[RestartStrategyAnnotation]
	if !isSet {
		return StrategySingle
	}
	return RestartStrategy(strategy)
}

func setPhaseAndStrategy(client k8s.Client, pod corev1.Pod, phase RestartPhase, strategy RestartStrategy) error {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[RestartPhaseAnnotation] = string(phase)
	pod.Annotations[RestartStrategyAnnotation] = string(strategy)
	return client.Update(&pod)
}

func removeAnnotations(client k8s.Client, pod corev1.Pod) error {
	delete(pod.Annotations, RestartPhaseAnnotation)
	delete(pod.Annotations, RestartStrategyAnnotation)
	return client.Update(&pod)
}

const (
	// ClusterRestartAnnotation can be set on the Elasticsearch cluster resource
	// to trigger a cluster restart. Its value must map a RestartStrategy.
	ClusterRestartAnnotation = "elasticsearch.k8s.elastic.co/restart"
)

func getClusterRestartAnnotation(cluster v1alpha1.Elasticsearch) RestartStrategy {
	if cluster.Annotations == nil {
		return RestartStrategy("")
	}
	return RestartStrategy(cluster.Annotations[ClusterRestartAnnotation])
}

func deleteClusterRestartAnnotation(client k8s.Client, cluster v1alpha1.Elasticsearch) error {
	delete(cluster.Annotations, ClusterRestartAnnotation)
	return client.Update(&cluster)
}
