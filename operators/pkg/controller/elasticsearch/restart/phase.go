package restart

import (
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

const (
	// TODO: comments
	RestartPhaseAnnotation = "elasticsearch.k8s.elastic.co/restart-phase"
)

type RestartPhase string

const (
	PhaseSchedule            RestartPhase = "schedule"
	PhaseScheduleCoordinated RestartPhase = "schedule-coordinated"

	//PhaseScheduleRolling     RestartPhase = "schedule-rolling"

	PhaseStop            RestartPhase = "stop"
	PhaseStopCoordinated RestartPhase = "stop-coordinated"

	PhaseStart            RestartPhase = "start"
	PhaseStartCoordinated RestartPhase = "start-coordinated"
)

func getPhase(pod corev1.Pod) (RestartPhase, bool) {
	phase, isSet := pod.Annotations[RestartPhaseAnnotation]
	return RestartPhase(phase), isSet
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

func removePhase(client k8s.Client, pod corev1.Pod) error {
	delete(pod.Annotations, RestartPhaseAnnotation)
	return client.Update(&pod)
}
