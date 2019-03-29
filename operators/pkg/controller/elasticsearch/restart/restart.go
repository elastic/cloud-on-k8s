package restart

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("mutation")

func HandlePodsReuse(client k8s.Client, changes mutation.Changes) (bool, error) {
	if changes.RequireFullClusterRestart {
		log.V(1).Info("changes requiring full cluster restart")
		// Schedule a coordinated restart on all pods to reuse
		if err := scheduleCoordinatedRestart(client, changes.ToReuse); err != nil {
			return false, err
		}
	}

	// build a list of pod with their target config
	pods := make(pod.PodsWithConfig, 0, len(changes.ToReuse)+len(changes.ToKeep))
	// including pods to reuse and their new config
	for _, p := range changes.ToReuse {
		pods = append(
			pods,
			pod.PodWithConfig{Pod: p.Initial.Pod, Config: p.Target.PodSpecCtx.Config},
		)
	}
	// and pods to keep: they already have the correct config but
	// may still be in the restart process
	for _, p := range changes.ToKeep {
		pods = append(pods, p)
	}

	// go through the restart state machine for each pod
	done, err := processPods(client, pods)
	if err != nil {
		return false, err
	}

	// consider done if no full cluster restart is required anymore,
	// and all pods have finished their restart process
	return !changes.RequireFullClusterRestart && done, nil
}

func processPods(client k8s.Client, pods pod.PodsWithConfig) (bool, error) {
	allDone := true
	for _, p := range pods {
		currentPhase, isSet := getPhase(p.Pod)
		if !isSet {
			// nothing to do: either not scheduled for restart or annotation not propagated yet
			continue
		}
		log.V(1).Info("Processing pod for reuse", "pod", p.Pod.Name)
		done, err := ExecutePhase(client, p, currentPhase, pods)
		if err != nil {
			return false, err
		}
		if !done {
			allDone = false
		}
	}

	return allDone, nil
}

func scheduleCoordinatedRestart(c k8s.Client, toReuse []mutation.PodToReuse) error {
	for _, p := range toReuse {
		pod := p.Initial.Pod
		phase, alreadySet := getPhase(pod)
		if alreadySet {
			log.V(1).Info(
				"Pod already in a restart phase",
				"pod", pod.Name,
				"phase", phase,
			)
			continue
		}
		if err := setPhase(c, pod, PhaseScheduleCoordinated); err != nil {
			return err
		}
	}
	return nil
}
