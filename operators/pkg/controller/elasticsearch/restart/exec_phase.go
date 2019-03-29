package restart

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
)

func ExecutePhase(
	client k8s.Client,
	pod pod.PodWithConfig,
	phase RestartPhase,
	allPods pod.PodsWithConfig,
) (done bool, err error) {
	switch phase {

	case PhaseSchedule:
		return false, execSchedule(client, pod)

	case PhaseScheduleCoordinated:
		return false, execScheduleCoordinated(client, pod)

	//case PhaseScheduleRolling:
	//	return execScheduleRolling(client, pod, allPods)

	case PhaseStop:
		return false, execStop(client, pod)

	case PhaseStopCoordinated:
		return false, execStopCoordinated(client, pod, allPods)

	case PhaseStart:
		return execStart(client, pod)

	case PhaseStartCoordinated:
		return execStartCoordinated(client, pod, allPods)

	default:
		return true, fmt.Errorf("unsupported restart phase: %s", phase)
	}
}

func execSchedule(client k8s.Client, p pod.PodWithConfig) error {
	return setPhase(client, p.Pod, PhaseStop)
}

func execScheduleCoordinated(client k8s.Client, p pod.PodWithConfig) error {
	return setPhase(client, p.Pod, PhaseStopCoordinated)
}

func execStop(client k8s.Client, p pod.PodWithConfig) error {
	// TODO
	// - disable shards allocations
	// - perform a sync flush
	// - stop the es process
	// - check if es stopped
	// - setPhase(client, p.Pod, PhaseStart)

	return setPhase(client, p.Pod, PhaseStart)
}

func execStopCoordinated(client k8s.Client, p pod.PodWithConfig, allPods pod.PodsWithConfig) error {
	// TODO
	// - disable shards allocations
	// - perform a sync flush
	// - stop the es process

	// - check if all es stopped <- retrieve from observers?

	return setPhase(client, p.Pod, PhaseStartCoordinated)
}

func execStart(client k8s.Client, p pod.PodWithConfig) (bool, error) {
	// TODO
	// - update pod configuration
	// - check if config is propagated
	// - Update pod labels?
	// - start es process
	// - check if es started
	// - enable shard allocations
	return true, removePhase(client, p.Pod)
}

func execStartCoordinated(client k8s.Client, p pod.PodWithConfig, allPods pod.PodsWithConfig) (bool, error) {
	// TODO
	// - update pod configuration
	// - check if config is propagated
	// - Update pod labels?
	// - start es process
	// - check if *all* ES started
	// - enable shard allocations

	return true, removePhase(client, p.Pod)
}
