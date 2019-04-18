// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

// Strategy specifies how to orchestrate restart on multiple pods.
type Strategy string

const (
	// ClusterRestartAnnotation can be set on the Elasticsearch cluster resource
	// to trigger a cluster restart. Its value must match an existing Strategy.
	ClusterRestartAnnotation = "elasticsearch.k8s.elastic.co/restart"

	// StrategyAnnotation, set on a pod, indicates the restart strategy to be used.
	StrategyAnnotation = "elasticsearch.k8s.elastic.co/restart-strategy"

	// StrategySingle schedules a simple restart.
	StrategySingle Strategy = "single"
	// StrategyCoordinated schedules a coordinated (simultaneous) restart.
	StrategyCoordinated Strategy = "coordinated"
	// StrategyRolling schedules a rolling (pod-by-pod) restart.
	StrategyRolling Strategy = "rolling"
)

// Phase represents a phase in the restart state machine.
type Phase string

const (
	// PhaseAnnotation, set on a pod, indicates the current phase of the underlying ES process.
	PhaseAnnotation = "elasticsearch.k8s.elastic.co/restart-phase"

	// PhaseSchedule indicates a restart is requested.
	PhaseSchedule Phase = "schedule"
	// PhaseSchedule indicates the ES process should be stopped.
	PhaseStop Phase = "stop"
	// PhaseSchedule indicates the ES process should be started.
	PhaseStart Phase = "start"
)

// Annotations helper functions

func getPhase(pod corev1.Pod) (Phase, bool) {
	phase, isSet := pod.Annotations[PhaseAnnotation]
	return Phase(phase), isSet
}

func hasPhase(pod corev1.Pod, expected Phase) bool {
	actual, isSet := getPhase(pod)
	return isSet && actual == expected
}

func isAnnotatedForRestart(pod corev1.Pod) bool {
	_, annotated := getPhase(pod)
	return annotated
}

func setPhase(client k8s.Client, pod corev1.Pod, phase Phase) error {
	log.V(1).Info(
		"Setting restart phase",
		"pod", pod.Name,
		"phase", phase,
	)
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[PhaseAnnotation] = string(phase)
	return client.Update(&pod)
}

func getStrategy(pod corev1.Pod) Strategy {
	strategy, isSet := pod.Annotations[StrategyAnnotation]
	if !isSet {
		return StrategySingle
	}
	return Strategy(strategy)
}

func setPhaseAndStrategy(client k8s.Client, pod corev1.Pod, phase Phase, strategy Strategy) error {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[PhaseAnnotation] = string(phase)
	pod.Annotations[StrategyAnnotation] = string(strategy)
	return client.Update(&pod)
}

func getClusterRestartAnnotation(cluster v1alpha1.Elasticsearch) Strategy {
	if cluster.Annotations == nil {
		return Strategy("")
	}
	return Strategy(cluster.Annotations[ClusterRestartAnnotation])
}

// AnnotateClusterForCoordinatedRestart annotates the given cluster to schedule
// a coordinated restart. The resource is not updated in the apiserver.
func AnnotateClusterForCoordinatedRestart(cluster *v1alpha1.Elasticsearch) {
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	cluster.Annotations[ClusterRestartAnnotation] = string(StrategyCoordinated)
}

func deletePodAnnotations(client k8s.Client, pod corev1.Pod) error {
	delete(pod.Annotations, PhaseAnnotation)
	delete(pod.Annotations, StrategyAnnotation)
	return client.Update(&pod)
}

func deleteClusterAnnotation(client k8s.Client, cluster v1alpha1.Elasticsearch) error {
	delete(cluster.Annotations, ClusterRestartAnnotation)
	return client.Update(&cluster)
}
