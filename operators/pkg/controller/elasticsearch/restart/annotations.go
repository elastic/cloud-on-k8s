// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

type RestartPhase string

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
	pod.Annotations[RestartPhaseAnnotation] = string(phase)
	return client.Update(&pod)
}

type RestartStrategy string

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
	pod.Annotations[RestartPhaseAnnotation] = string(phase)
	pod.Annotations[RestartStrategyAnnotation] = string(strategy)
	return client.Update(&pod)
}

func removeAnnotations(client k8s.Client, pod corev1.Pod) error {
	delete(pod.Annotations, RestartPhaseAnnotation)
	delete(pod.Annotations, RestartStrategyAnnotation)
	return client.Update(&pod)
}
