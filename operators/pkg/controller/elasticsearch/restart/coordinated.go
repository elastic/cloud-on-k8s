// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"context"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
)

func scheduleCoordinatedRestart(c k8s.Client, toReuse []mutation.PodToReuse) (int, error) {
	count := 0
	for _, p := range toReuse {
		pod := p.Initial.Pod
		if isAnnotatedForRestart(pod) {
			log.V(1).Info(
				"Pod already in a restart phase",
				"pod", pod.Name,
			)
			continue
		}
		if err := setPhaseAndStrategy(c, pod, PhaseSchedule, StrategyCoordinated); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

type CoordinatedRestart struct {
	k8sClient k8s.Client
	esClient  client.Client
	cluster   v1alpha1.Elasticsearch
	pods      pod.PodsWithConfig
}

type Step struct {
	startPhase RestartPhase
	endPhase   RestartPhase
	do         func(pods pod.PodsWithConfig) (bool, error)
}

func (c *CoordinatedRestart) Exec() (bool, error) {
	if len(c.pods) == 0 {
		return true, nil
	}
	for _, step := range []Step{
		c.scheduleStop(),
		c.stop(),
		c.start(),
		c.finalizeRestart(),
	} {
		pods := filterPodsInPhase(c.pods, step.startPhase)
		if len(pods) == 0 {
			continue // all pods are past this step
		}
		// apply step on pods matching the start phase
		done, err := step.do(pods)
		if err != nil {
			return false, err
		}
		if !done {
			// requeue
			return false, nil
		}
		// check if all pods have reached the end phase for this step
		if len(filterPodsInPhase(c.pods, step.endPhase)) != len(c.pods) {
			// some pods have not reached the end phase for this step yet, requeue
			return false, nil
		}
	}
	log.Info("Coordinated restart successful")
	return true, nil
}

// scheduleStop annotates all pods in the "stop" phase.
func (c *CoordinatedRestart) scheduleStop() Step {
	return Step{
		startPhase: PhaseSchedule,
		endPhase:   PhaseStop,
		do: func(pods pod.PodsWithConfig) (bool, error) {
			if err := c.setPhase(pods, PhaseStop); err != nil {
				return false, err
			}
			return true, nil
		},
	}
}

func (c *CoordinatedRestart) stop() Step {
	return Step{
		startPhase: PhaseStop,
		endPhase:   PhaseStart,
		do: func(pods pod.PodsWithConfig) (bool, error) {
			if err := c.prepareClusterForStop(); err != nil {
				return false, err
			}

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

func (c *CoordinatedRestart) start() Step {
	return Step{
		startPhase: PhaseStop,
		endPhase:   PhaseStart,
		do: func(pods pod.PodsWithConfig) (bool, error) {
			podsDone := 0
			for _, p := range pods {
				// update pod configuration
				done, err := c.ensureConfigurationUpdated(p)
				if err != nil {
					return false, err
				}
				if !done {
					continue
				}

				// - Update pod labels?
				// - ensure es process started
				c.ensureESProcessStarted(p)
				podsDone += 1
			}

			if podsDone != len(pods) {
				return false, nil // requeue
			}

			// re-enable shard allocation
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			if err := c.esClient.EnableShardAllocation(ctx); err != nil {
				return false, err
			}

			return true, nil
		},
	}
}

func (c *CoordinatedRestart) finalizeRestart() Step {
	return Step{
		startPhase: PhaseStart,
		endPhase:   "",
		do: func(pods pod.PodsWithConfig) (bool, error) {
			for _, p := range pods {
				if err := removeAnnotations(c.k8sClient, p.Pod); err != nil {
					return false, err
				}
			}
			return true, nil
		},
	}
}

func (c *CoordinatedRestart) setPhase(pods pod.PodsWithConfig, phase RestartPhase) error {
	for _, p := range pods {
		if err := setPhase(c.k8sClient, p.Pod, phase); err != nil {
			return err
		}
	}
	return nil
}

func (c *CoordinatedRestart) prepareClusterForStop() error {
	// disable shard allocation to ensure shards from the restarted node
	// won't be moved around
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	if err := c.esClient.DisableShardAllocation(ctx); err != nil {
		return err
	}

	// perform a synced flush (best effort) to speedup shard recovery
	ctx2, cancel2 := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel2()
	if err := c.esClient.SyncedFlush(ctx2); err != nil {
		return err
	}

	return nil
}

func (c *CoordinatedRestart) ensureESProcessStopped(p pod.PodsWithConfig) (bool, error) {
	// TODO
	return true, nil
}

func (c *CoordinatedRestart) ensureESProcessStarted(p pod.PodWithConfig) (bool, error) {
	// TODO
	return true, nil
}

func (c *CoordinatedRestart) ensureConfigurationUpdated(p pod.PodWithConfig) (bool, error) {
	if err := settings.ReconcileConfig(c.k8sClient, c.cluster, p.Pod, p.Config); err != nil {
		return false, err
	}
	// TODO
	// compute config checksum

	// retrieve config as seen by the process manager

	// requeue if not equal

	return true, nil
}

func filterPodsInPhase(pods pod.PodsWithConfig, phase RestartPhase) pod.PodsWithConfig {
	filtered := make(pod.PodsWithConfig, 0, len(pods))
	for _, p := range pods {
		if hasPhase(p.Pod, phase) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
