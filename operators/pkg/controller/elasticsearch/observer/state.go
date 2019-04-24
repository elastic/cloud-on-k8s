// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"sync"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	netutils "github.com/elastic/k8s-operators/operators/pkg/utils/net"
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
	ctx context.Context, k8sClient k8s.Client,
	cluster types.NamespacedName, esClient esclient.Client,
	caCerts []*x509.Certificate, dialer netutils.Dialer,
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
		for i, pod := range pods {
			wg.Add(1)
			go func(idx int, p corev1.Pod) {
				defer wg.Done()
				status := getKeystoreStatus(ctx, caCerts, dialer, p)
				keystoreStatuses[idx] = status
			}(i, pod)
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

func getKeystoreStatus(ctx context.Context, caCerts []*x509.Certificate, dialer netutils.Dialer, pod corev1.Pod) keystore.Status {
	if !k8s.IsPodReady(pod) {
		log.V(3).Info("Pod not ready to retrieve keystore status", "name", pod.Name)
		return keystore.Status{State: keystore.WaitingState}
	}

	podIP := net.ParseIP(pod.Status.PodIP)
	endpoint := fmt.Sprintf("https://%s:%d", podIP.String(), processmanager.DefaultPort)
	pmClient := processmanager.NewClient(endpoint, caCerts, dialer)
	status, err := pmClient.KeystoreStatus(ctx)
	if err != nil {
		log.V(3).Info("Unable to retrieve keystore status", "name", pod.Name, "error", err)
		return keystore.Status{State: keystore.State("Unreachable")}
	}

	log.V(3).Info("Keystore updater", "name", pod.Name, "status", status)
	return status
}
