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
		// Schedule a full cluster restart before doing any other changes
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
	// and pods to keep (already have the correct config)
	for _, p := range changes.ToKeep {
		pods = append(pods, p)
	}

	return processPods(client, pods)
}

func processPods(client k8s.Client, pods pod.PodsWithConfig) (bool, error) {
	countDone := 0
	for _, p := range pods {
		log.V(1).Info("Processing pod for reuse", "pod", p.Pod.Name)
		currentPhase, isSet := getPhase(p.Pod)
		if !isSet {
			// nothing to do (yet) for this pod, first annotation isn't yet propagated
			continue
		}
		done, err := ExecutePhase(client, p, currentPhase, pods)
		if err != nil {
			return false, err
		}
		if done {
			countDone++
		}
	}

	return countDone == len(pods), nil
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
