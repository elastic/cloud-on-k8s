// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
)

// CoordinatedRestartDefaultTimeout is the time after which we consider the coordinated restart is
// taking too long to proceed and probably won't complete
const CoordinatedRestartDefaultTimeout = 15 * time.Minute

// scheduleCoordinatedRestart annotates all pods for a coordinated restart.
func scheduleCoordinatedRestart(c k8s.Client, pods pod.PodsWithConfig) (int, error) {
	count := 0
	for _, p := range pods {
		if isAnnotatedForRestart(p.Pod) {
			log.V(1).Info("Pod already in a restart phase", "pod", p.Pod.Name)
			continue
		}
		if err := setScheduleRestartAnnotations(c, p.Pod, StrategyCoordinated, time.Now()); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// CoordinatedRestart holds the logic to restart nodes simultaneously.
// It waits for all nodes to be stopped, then starts them all.
type CoordinatedRestart struct {
	RestartContext
	pods            []corev1.Pod
	timeout         time.Duration
	pmClientFactory pmClientFactory
}

func NewCoordinatedRestart(restartContext RestartContext) *CoordinatedRestart {
	return &CoordinatedRestart{
		RestartContext:  restartContext,
		timeout:         CoordinatedRestartDefaultTimeout,
		pmClientFactory: createProcessManagerClient,
	}
}

// Exec attempts some progression on the restart process for all pods.
func (c *CoordinatedRestart) Exec() (done bool, err error) {
	// All pods will go simultaneously through each of the following steps
	return c.coordinatedStepsExec(
		// prepare the cluster to be stopped
		c.scheduleStop(),
		// stop all ES processes
		c.stop(),
		// start all ES processes
		c.start(),
	)
}

// Step specifies an action to apply on all pods in the initPhase.
// Once the action is over, pods should have the endPhase applied.
type Step struct {
	initPhase Phase
	endPhase  Phase
	do        func(pods []corev1.Pod) (bool, error)
}

// coordinatedStepsExec executes a series of step in a coordinated way.
//
// The restart process is implemented as a state machine, persisted through Phases in pod annotations.
// Pods move from one step to another at the same time: we wait until all pods have completed one step
// until we can move over to the next step.
// Each phase is idempotent: it's ok to repeat the same phase several times (multiple reconciliations).
// In many cases we exit early with 'false': the complete restart cannot be achieved in a single
// call, but we'll eventually make some progress.
func (c *CoordinatedRestart) coordinatedStepsExec(steps ...Step) (done bool, err error) {
	if len(c.pods) == 0 {
		// nothing to do
		return true, nil
	}

	// abort the restart for pods which have reached timeout
	for i, p := range c.pods {
		aborted, err := c.abortIfTimeoutReached(p)
		if err != nil {
			return false, err
		}
		if aborted {
			// no need to keep this pod
			c.pods = append(c.pods[:i], c.pods[i+1:]...)
		}
	}

	log.Info("Handling coordinated restart", "count", len(c.pods))

	for _, step := range steps {
		pods := filterPodsInPhase(c.pods, step.initPhase)
		if len(pods) == 0 {
			continue // all pods are past this step, move on to next step
		}
		// apply step on matching pods
		done, err := step.do(pods)
		if err != nil {
			return false, err
		}
		if !done {
			// step not over yet for some pods: requeue
			return false, nil
		}
		if len(filterPodsInPhase(c.pods, step.endPhase)) != len(c.pods) {
			// all pods are over this step, but not annotated yet with the next phase: requeue
			return false, nil
		}
	}

	return true, nil
}

// scheduleStop prepares the cluster to be stopped, then moves pods to the "stop" phase.
func (c *CoordinatedRestart) scheduleStop() Step {
	return Step{
		initPhase: PhaseSchedule,
		endPhase:  PhaseStop,
		do: func(pods []corev1.Pod) (bool, error) {
			if err := c.prepareClusterForStop(); err != nil {
				// We consider this call best-effort: ES endpoint might not be reachable.
				// Let's continue.
				log.Error(err, "Failed to prepare the cluster for full restart (might not be reachable). Continuing.")
			}
			if err := c.setPhase(pods, PhaseStop); err != nil {
				return false, err
			}
			return true, nil
		},
	}
}

// stop ensures all ES processes are stopped, then move pods to the "start" phase.
func (c *CoordinatedRestart) stop() Step {
	return Step{
		initPhase: PhaseStop,
		endPhase:  PhaseStart,
		do: func(pods []corev1.Pod) (bool, error) {
			allStopped, err := c.ensureESProcessStopped(pods)
			if err != nil {
				return false, err
			}
			if !allStopped {
				return false, nil // requeue
			}

			if err := c.setPhase(pods, PhaseStart); err != nil {
				return false, err
			}

			return true, nil
		},
	}
}

// start ensures that:
// - all ES processes are started
// - shards allocation is enabled again
// Then removes the restart annotation from all pods.
func (c *CoordinatedRestart) start() Step {
	return Step{
		initPhase: PhaseStart,
		endPhase:  "",
		do: func(pods []corev1.Pod) (bool, error) {
			podsDone := 0
			for _, p := range pods {
				// - ensure es process started
				started, err := c.ensureESProcessStarted(p)
				if err != nil {
					return false, err
				}
				if started {
					podsDone++
				}
			}

			if podsDone != len(pods) {
				log.V(1).Info("Some pods are not started yet", "expected", len(pods), "actual", podsDone)
				return false, nil // requeue
			}

			externalService, err := services.GetExternalService(c.K8sClient, c.Cluster)
			if err != nil {
				return false, err
			}
			esReachable, err := services.IsServiceReady(c.K8sClient, externalService)
			if err != nil {
				return false, err
			}
			if !esReachable {
				log.V(1).Info("Cluster is not ready to receive requests yet")
				return false, nil // requeue
			}

			// re-enable shard allocation
			log.V(1).Info("Enabling shards allocation")
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			if err := c.EsClient.EnableShardAllocation(ctx); err != nil {
				return false, err
			}

			// restart is over, remove all annotations
			for _, p := range pods {
				if err := deletePodAnnotations(c.K8sClient, p); err != nil {
					return false, err
				}
			}

			c.EventsRecorder.AddEvent(
				corev1.EventTypeNormal, events.EventReasonRestart,
				fmt.Sprintf("Coordinated restart complete for cluster %s", c.Cluster.Name),
			)
			log.Info("Coordinated restart complete", "cluster", c.Cluster.Name)

			return true, nil
		},
	}
}

// setPhase applies the given phase to the given pods.
func (c *CoordinatedRestart) setPhase(pods []corev1.Pod, phase Phase) error {
	for _, p := range pods {
		if err := setPhase(c.K8sClient, p, phase); err != nil {
			return err
		}
	}
	return nil
}

// prepareClusterForStop performs cluster-wide ES requests to speedup the restart process.
// See https://www.elastic.co/guide/en/elasticsearch/reference/6.7/restart-upgrade.html.
func (c *CoordinatedRestart) prepareClusterForStop() error {
	// disable shard allocation to ensure shards from stopped nodes
	// won't be moved around during the restart process
	log.V(1).Info("Disabling shards allocation for coordinated restart")
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := c.EsClient.DisableShardAllocation(ctx); err != nil {
		return err
	}

	// perform a synced flush (best effort) to speedup shard recovery
	// any ongoing indexing operation on a particular shard will make the sync flush
	// fail for that particular shard, that's ok.
	ctx2, cancel2 := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel2()
	if err := c.EsClient.SyncedFlush(ctx2); err != nil {
		return err
	}

	return nil
}

// ensureESProcessStopped interacts with the process manager to stop the ES process.
func (c *CoordinatedRestart) ensureESProcessStopped(pods []corev1.Pod) (bool, error) {
	stoppedCount := 0
	// TODO: parallel requests
	for _, p := range pods {
		// request ES process stop through the pod's process manager (idempotent)
		pmClient, err := c.pmClientFactory(c.RestartContext, p)
		if err != nil {
			return false, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
		defer cancel()
		log.V(1).Info("Requesting ES process stop", "pod", p.Name)
		status, err := pmClient.Stop(ctx)
		if err != nil {
			return false, err
		}
		// we got the current status back, check if the process is stopped
		if status.State == processmanager.Stopped {
			log.V(1).Info("ES process successfully stopped", "pod", p.Name)
			stoppedCount++
		} else {
			log.V(1).Info("ES process is not stopped yet", "pod", p.Name, "state", status.State)
		}
	}
	return stoppedCount == len(pods), nil
}

// ensureESProcessStarted interacts with the process manager to ensure all ES processes are started.
func (c *CoordinatedRestart) ensureESProcessStarted(p corev1.Pod) (bool, error) {
	pmClient, err := c.pmClientFactory(c.RestartContext, p)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
	defer cancel()
	log.V(1).Info("Requesting ES process start", "pod", p.Name)
	status, err := pmClient.Start(ctx)
	if err != nil {
		return false, err
	}

	// check the returned process status
	if status.State != processmanager.Started {
		log.V(1).Info("ES process is not started yet", "pod", p.Name, "state", status.State)
		// not started yet, requeue
		return false, nil
	}

	log.V(1).Info("ES process successfully started", "pod", p.Name)
	return true, nil
}

// getESStatus returns the current ES process status in the given pod.
func (c *CoordinatedRestart) getESStatus(p corev1.Pod) (processmanager.ProcessState, error) {
	pmClient, err := c.pmClientFactory(c.RestartContext, p)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
	defer cancel()
	status, err := pmClient.Status(ctx)
	if err != nil {
		return "", err
	}

	return status.State, nil
}

func (c *CoordinatedRestart) abortIfTimeoutReached(pod corev1.Pod) (bool, error) {
	startTime, isSet := getStartTime(pod)
	if !isSet {
		// start time doesn't appear in the cache yet, or has been tweaked by a human
		return false, nil
	}
	if time.Now().Sub(startTime) > c.timeout {
		log.Error(
			errors.New("timeout exceeded"), "Coordinated restart is taking too long, aborting.",
			"pod", pod.Name, "timeout", c.timeout,
		)
		// We've reached the restart timeout for this pod: chances are something is wrong and a human
		// intervention is required to figure it out.
		// We don't want to block the reconciliation loop on the restart forever: going forward with the
		// reconciliation might actually fix the current situation.
		// Let's abort the restart by removing restart annotations.
		// The pod is left in an unknown state.
		if err := deletePodAnnotations(c.K8sClient, pod); err != nil {
			return false, err
		}
		c.EventsRecorder.AddEvent(
			corev1.EventTypeWarning, events.EventReasonUnexpected,
			fmt.Sprintf("Aborting coordinated restart for pod %s, timeout exceeded.", pod.Name),
		)
		return true, nil
	}
	return false, nil
}
