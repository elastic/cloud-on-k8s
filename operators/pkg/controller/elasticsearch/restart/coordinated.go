// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"context"
	"fmt"
	"hash/crc32"
	"net"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	netutils "github.com/elastic/k8s-operators/operators/pkg/utils/net"
	corev1 "k8s.io/api/core/v1"
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
	dialer    netutils.Dialer
	cluster   v1alpha1.Elasticsearch
	pods      pod.PodsWithConfig
}

type Step struct {
	initPhase RestartPhase
	endPhase  RestartPhase
	do        func(pods pod.PodsWithConfig) (bool, error)
}

func (c *CoordinatedRestart) Exec() (bool, error) {
	if len(c.pods) == 0 {
		return true, nil
	}
	log.Info("Handling coordinated restart", "count", len(c.pods))
	for _, step := range []Step{
		c.scheduleStop(),
		c.stop(),
		c.start(),
	} {
		pods := filterPodsInPhase(c.pods, step.initPhase)
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
		initPhase: PhaseSchedule,
		endPhase:  PhaseStop,
		do: func(pods pod.PodsWithConfig) (bool, error) {
			if err := c.prepareClusterForStop(); err != nil {
				return false, err
			}
			if err := c.setPhase(pods, PhaseStop); err != nil {
				return false, err
			}
			return true, nil
		},
	}
}

func (c *CoordinatedRestart) stop() Step {
	return Step{
		initPhase: PhaseStop,
		endPhase:  PhaseStart,
		do: func(pods pod.PodsWithConfig) (bool, error) {
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
		initPhase: PhaseStart,
		endPhase:  "",
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
				started, err := c.ensureESProcessStarted(p)
				if err != nil {
					return false, err
				}
				if started {
					podsDone++
				}
			}

			if podsDone != len(pods) {
				return false, nil // requeue
			}

			// re-enable shard allocation
			log.V(1).Info("Enabling shards allocation")
			ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
			defer cancel()
			if err := c.esClient.EnableShardAllocation(ctx); err != nil {
				return false, err
			}

			// restart over, remove all annotations
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
	log.V(1).Info("Disabling shards allocation for coordinated restart")
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

func (c *CoordinatedRestart) ensureESProcessStopped(pods pod.PodsWithConfig) (bool, error) {
	stoppedCount := 0
	// TODO: parallel requests
	for _, p := range pods {
		// request ES process stop through the pod's process manager (idempotent)
		pmClient, err := c.processManagerClient(p.Pod)
		if err != nil {
			return false, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
		defer cancel()
		log.V(1).Info("Requesting ES process stop", "pod", p.Pod.Name)
		status, err := pmClient.Stop(ctx)
		if err != nil {
			return false, err
		}
		// we got the current status back, check if the process is stopped
		if status.State == processmanager.Stopped {
			log.V(1).Info("ES process successfully stopped", "pod", p.Pod.Name)
			stoppedCount++
		} else {
			log.V(1).Info("ES process is not stopped yet", "pod", p.Pod.Name, "state", status.State)
		}
	}
	return stoppedCount == len(pods), nil
}

func (c *CoordinatedRestart) ensureESProcessStarted(p pod.PodWithConfig) (bool, error) {
	// request ES process stop through the pod's process manager (idempotent)
	pmClient, err := c.processManagerClient(p.Pod)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
	defer cancel()
	status, err := pmClient.Start(ctx)
	if err != nil {
		return false, err
	}

	// check the returned process status
	if status.State != processmanager.Started {
		log.V(1).Info("ES process is not started yet", "pod", p.Pod.Name, "state", status.State)
		// not started yet, requeue
		return false, nil
	}

	log.V(1).Info("ES process successfully started", "pod", p.Pod.Name)
	return true, nil
}

func (c *CoordinatedRestart) ensureConfigurationUpdated(p pod.PodWithConfig) (bool, error) {
	log.Info(fmt.Sprintf("config to reconcile: %s", p.Config.Render()))
	if err := settings.ReconcileConfig(c.k8sClient, c.cluster, p.Pod, p.Config); err != nil {
		return false, err
	}

	// retrieve config as seen by the process manager
	pmcClient, err := c.processManagerClient(p.Pod)
	if err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
	defer cancel()
	status, err := pmcClient.Status(ctx)
	if err != nil {
		return false, err
	}

	expectedConfigChecksum := fmt.Sprint(crc32.ChecksumIEEE(p.Config.Render()))

	// compare expected config with config as seen from the process manager
	if status.ConfigChecksum != expectedConfigChecksum {
		log.V(1).Info("Configuration is not propagated yet, checksum mismatch", "pod", p.Pod.Name)
		return false, nil
	}

	log.V(1).Info("Configuration is correctly propagated to the pod", "pod", p.Pod.Name)
	return true, nil
}

func (c *CoordinatedRestart) getESStatus(p corev1.Pod) (processmanager.ProcessState, error) {
	pmClient, err := c.processManagerClient(p)
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

func (c *CoordinatedRestart) processManagerClient(pod corev1.Pod) (*processmanager.Client, error) {
	podIP := net.ParseIP(pod.Status.PodIP)
	url := fmt.Sprintf("https://%s:%d", podIP.String(), processmanager.DefaultPort)
	rawCA, err := nodecerts.GetCA(c.k8sClient, k8s.ExtractNamespacedName(&c.cluster.ObjectMeta))
	if err != nil {
		return nil, err
	}
	certs, err := certificates.ParsePEMCerts(rawCA)
	if err != nil {
		return nil, err
	}
	return processmanager.NewClient(url, certs, c.dialer), nil
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
