package restart

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	netutils "github.com/elastic/k8s-operators/operators/pkg/utils/net"
)

var log = logf.Log.WithName("restart")

// RestartContext groups together dependencies required to perform ES restarts.
type RestartContext struct {
	K8sClient      k8s.Client
	EsClient       client.Client
	EventsRecorder *events.Recorder
	Dialer         netutils.Dialer
	Cluster        v1alpha1.Elasticsearch
	Changes        mutation.Changes
}

// HandleESRestarts will attempt progression for ES processes that should be restarted.
func HandleESRestarts(restartContext RestartContext) (done bool, err error) {
	// start with any restart currently in progress
	done, err = processOngoingRestarts(restartContext)
	if err != nil {
		return false, err
	}
	if !done {
		// restart is not over yet, requeue
		return false, err
	}

	// no ongoing restart, are there other restarts to schedule?
	annotatedCount, err := scheduleRestarts(restartContext)
	if err != nil {
		return false, err
	}
	if annotatedCount != 0 {
		// we did annotate some pods for restart, let's requeue until all annotations
		// are propagated to the resources cache for the restart to kick off
		return false, nil
	}

	// nothing to do
	return true, nil
}

// processOngoingRestarts attempts to progress the restart state machine of concerned pods.
func processOngoingRestarts(restartContext RestartContext) (done bool, err error) {
	// TODO: include changes.ToReuse here
	podsToLookAt := restartContext.Changes.ToKeep

	// find all pods currently in a restart process, and group them by restart strategy
	annotatedPods := map[Strategy]pod.PodsWithConfig{}
	for _, p := range podsToLookAt {
		if isAnnotatedForRestart(p.Pod) {
			strategy := getStrategy(p.Pod)
			if _, exists := annotatedPods[strategy]; !exists {
				annotatedPods[strategy] = pod.PodsWithConfig{}
			}
			annotatedPods[strategy] = append(annotatedPods[strategy], p)
		}
	}

	if len(annotatedPods) == 0 {
		// no restart to process
		return true, nil
	}

	log.V(1).Info("Pods annotated for restart", "count", len(annotatedPods))

	if len(annotatedPods[StrategyCoordinated]) > 0 {
		// run the coordinated restart
		restart := &CoordinatedRestart{
			RestartContext: restartContext,
			pods:           annotatedPods[StrategyCoordinated],
		}
		done, err = restart.Exec()
	}

	return done, err
}

// scheduleRestarts inspects the current cluster and changes, to maybe annotate some pods for restart.
func scheduleRestarts(restartContext RestartContext) (int, error) {
	// a coordinated restart can be requested at the cluster-level
	if getClusterRestartAnnotation(restartContext.Cluster) == StrategyCoordinated {
		// annotate all current pods of the cluster (toKeep)
		// we don't care about pods to create or pods to delete here
		// TODO: include changes.ToReuse here
		count, err := scheduleCoordinatedRestart(restartContext.K8sClient, restartContext.Changes.ToKeep)
		if err != nil {
			return 0, err
		}
		// pods are now annotated: remove annotation from the cluster
		// to avoid restarting over and over again
		if err := deleteClusterAnnotation(restartContext.K8sClient, restartContext.Cluster); err != nil {
			return 0, err
		}
		restartContext.EventsRecorder.AddEvent(
			corev1.EventTypeNormal, events.EventReasonRestart,
			fmt.Sprintf("Coordinated restart scheduled for cluster %s", restartContext.Cluster.Name),
		)
		return count, nil
	}

	return 0, nil
}
