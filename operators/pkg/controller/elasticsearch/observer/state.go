// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"context"
	"sync"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// State contains information about an observed state of Elasticsearch.
type State struct {
	// TODO: verify usages of the two below never assume they are set (check for nil)

	// ClusterState is the current Elasticsearch cluster state if any.
	ClusterState *esclient.ClusterState
	// ClusterHealth is the current traffic light health as reported by Elasticsearch.
	ClusterHealth *esclient.Health
	// TODO should probably be a separate observer
	// ClusterLicense is the current license applied to this cluster
	ClusterLicense *esclient.License
	// KeystoreStatuses are the status of the keystore updater of each pods
	KeystoreStatuses []keystore.Status
}

// RetrieveState returns the current Elasticsearch cluster state
func RetrieveState(
	ctx context.Context,
	cluster types.NamespacedName, esClient esclient.Client,
	k8sClient k8s.Client, pmClientFactory pmClientFactory,
) State {
	// retrieve both cluster state and health in parallel
	clusterStateChan := make(chan *client.ClusterState)
	healthChan := make(chan *client.Health)
	licenseChan := make(chan *client.License)
	keystoreStatusesChan := make(chan []keystore.Status)

	go func() {
		clusterState, err := esClient.GetClusterState(ctx)
		if err != nil {
			log.V(3).Info("Unable to retrieve cluster state", "error", err)
			clusterStateChan <- nil
			return
		}
		clusterStateChan <- &clusterState
	}()

	go func() {
		health, err := esClient.GetClusterHealth(ctx)
		if err != nil {
			log.V(3).Info("Unable to retrieve cluster health", "error", err)
			healthChan <- nil
			return
		}
		healthChan <- &health
	}()

	go func() {
		license, err := esClient.GetLicense(ctx)
		if err != nil {
			log.V(3).Info("Unable to retrieve cluster license", "error", err)
			licenseChan <- nil
			return
		}
		licenseChan <- &license
	}()

	go func() {
		// fetch pods
		labelSelector := label.NewLabelSelectorForElasticsearchClusterName(cluster.Name)
		pods, err := k8s.GetPods(k8sClient, cluster.Namespace, labelSelector, nil)
		if err != nil {
			keystoreStatusesChan <- nil
			return
		}

		keystoreStatuses := make([]keystore.Status, len(pods))
		wg := sync.WaitGroup{}
		// request the process manager API for each pod
		for i, p := range pods {
			wg.Add(1)
			go func(idx int, pod corev1.Pod) {
				defer wg.Done()
				status := getKeystoreStatus(ctx, pmClientFactory, pod)
				keystoreStatuses[idx] = status
			}(i, p)
		}
		wg.Wait()
		keystoreStatusesChan <- keystoreStatuses
	}()

	// return the state when ready, may contain nil values
	return State{
		ClusterHealth:    <-healthChan,
		ClusterState:     <-clusterStateChan,
		ClusterLicense:   <-licenseChan,
		KeystoreStatuses: <-keystoreStatusesChan,
	}
}

func getKeystoreStatus(ctx context.Context, pmClientFactory pmClientFactory, pod corev1.Pod) keystore.Status {
	if !k8s.IsPodReady(pod) {
		log.V(3).Info("Pod not ready to retrieve keystore status", "pod_name", pod.Name)
		return keystore.Status{State: keystore.WaitingState, Reason: "Pod not ready"}
	}

	status, err := pmClientFactory().KeystoreStatus(ctx)
	if err != nil {
		log.V(3).Info("Unable to retrieve keystore status", "pod_name", pod.Name, "error", err)
		return keystore.Status{State: keystore.FailedState, Reason: "Unable to retrieve keystore status"}
	}

	log.V(3).Info("Keystore updater", "pod_name", pod.Name, "status", status)
	return status
}
