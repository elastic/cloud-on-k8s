// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("mutation")

func HandlePodsReuse(k8sClient k8s.Client, esClient client.Client, dialer net.Dialer, cluster v1alpha1.Elasticsearch, changes mutation.Changes) (done bool, err error) {
	annotatedCount, err := annotateForRestart(k8sClient, cluster, changes)
	if err != nil {
		return false, err
	}
	if annotatedCount != 0 {
		// we did annotate some pods for reuse, let's requeue until all annotations
		// are propagated to our resources cache
		return false, nil
	}

	// no more pods to annotate, let's process annotated ones
	return processRestarts(k8sClient, esClient, dialer, cluster, changes)
}

func annotateForRestart(client k8s.Client, cluster v1alpha1.Elasticsearch, changes mutation.Changes) (count int, err error) {
	if changes.RequireFullClusterRestart {
		// Schedule a coordinated restart on all pods to reuse
		log.V(1).Info("Changes requiring full cluster restart")
		pods := make([]corev1.Pod, len(changes.ToReuse))
		for _, p := range changes.ToReuse {
			pods = append(pods, p.Initial.Pod)
		}
		return scheduleCoordinatedRestart(client, pods)
	}

	if getClusterRestartAnnotation(cluster) == StrategyCoordinated {
		// Schedule a coordinated restart on all pods of the cluster (to keep + to reuse)
		pods := make([]corev1.Pod, 0, len(changes.ToReuse)+len(changes.ToKeep))
		for _, p := range changes.ToKeep {
			pods = append(pods, p.Pod)
		}
		for _, p := range changes.ToReuse {
			pods = append(pods, p.Initial.Pod)
		}

		// annotate all pods
		count, err := scheduleCoordinatedRestart(client, pods)
		if err != nil {
			return 0, err
		}
		// remove annotation from the cluster, to avoid restarting over and over again
		if err := deleteClusterRestartAnnotation(client, cluster); err != nil {
			return 0, err
		}

		return count, nil
	}

	return 0, nil
}

func processRestarts(k8sClient k8s.Client, esClient client.Client, dialer net.Dialer, cluster v1alpha1.Elasticsearch, changes mutation.Changes) (done bool, err error) {

	// both pods to keep and pods to reuse may be annotated for restart
	podsToLookAt := make(pod.PodsWithConfig, len(changes.ToKeep)+len(changes.ToReuse))
	copy(podsToLookAt, changes.ToKeep)
	for _, p := range changes.ToReuse {
		podsToLookAt = append(
			podsToLookAt,
			// for pods reuse include the target config, not the initial one
			pod.PodWithConfig{Pod: p.Initial.Pod, Config: p.Target.PodSpecCtx.Config},
		)
	}

	// group them by restart strategy
	annotatedPods := map[RestartStrategy]pod.PodsWithConfig{}
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
		return true, nil
	}

	log.V(1).Info("Pods annotated for restart", "count", len(annotatedPods))

	// run the restarts
	coordinated := CoordinatedRestart{
		k8sClient: k8sClient,
		esClient:  esClient,
		dialer:    dialer,
		cluster:   cluster,
		pods:      annotatedPods[StrategyCoordinated],
	}

	return coordinated.Exec()
}
