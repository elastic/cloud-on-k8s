// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"context"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("remotecluster")
)

// UpdateRemoteCluster updates the remote clusters in the persistent settings by calling the Elasticsearch API.
func UpdateRemoteCluster(
	c k8s.Client,
	esClient esclient.Client,
	es v1alpha1.Elasticsearch,
	reconcileState *reconcile.State,
) error {

	currentRemoteClusters := reconcileState.GetRemoteClusters()
	if currentRemoteClusters == nil {
		currentRemoteClusters = make(map[string]string)
	}
	expectedRemoteClusters, err := getRemoteClusters(c, es.Name, es.Namespace)
	if err != nil {
		return err
	}

	// RemoteClusters to add
	for name, remoteCluster := range expectedRemoteClusters {
		if version, ok := currentRemoteClusters[name]; !ok || remoteCluster.ResourceVersion != version {
			// Make a copy of the array
			seedHosts := make([]string, len(remoteCluster.Status.SeedHosts))
			copy(seedHosts, remoteCluster.Status.SeedHosts)
			// Declare remote cluster in ES
			persistentSettings := newRemoteClusterSetting(name, seedHosts)
			log.V(1).Info("Add new remote cluster",
				"localCluster", es.Name,
				"remoteCluster", remoteCluster.Name,
				"seeds", seedHosts,
			)
			err := updateRemoteCluster(esClient, persistentSettings)
			if err != nil {
				return nil
			}
			currentRemoteClusters[name] = remoteCluster.ResourceVersion
		}
	}

	// RemoteClusters to remove
	for name := range currentRemoteClusters {
		if _, ok := expectedRemoteClusters[name]; !ok {
			persistentSettings := newRemoteClusterSetting(name, nil)
			log.V(1).Info("Remove remote cluster",
				"localCluster", es.Name,
				"remoteCluster", name,
			)
			err := updateRemoteCluster(esClient, persistentSettings)
			if err != nil {
				return nil
			}
			delete(currentRemoteClusters, name)
		}
	}
	// Update state
	reconcileState.UpdateRemoteClusters(currentRemoteClusters)
	return nil
}

// getRemoteClusters loads the trust relationships from the API.
func getRemoteClusters(c k8s.Client, clusterName, namespace string) (map[string]v1alpha1.RemoteCluster, error) {
	var remoteClusterList v1alpha1.RemoteClusterList
	if err := c.List(&client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{label.ClusterNameLabelName: clusterName}),
		Namespace:     namespace,
	}, &remoteClusterList); err != nil {
		return nil, err
	}

	remoteClusters := make(map[string]v1alpha1.RemoteCluster, len(remoteClusterList.Items))
	for _, remoteCluster := range remoteClusterList.Items {
		remoteClusters[remoteCluster.Name] = remoteCluster
	}
	log.V(1).Info(
		"Loaded remote clusters",
		"clusterName", clusterName,
		"count", len(remoteClusterList.Items),
	)

	return remoteClusters, nil
}

// newRemoteClusterSetting creates a persistent setting to add or remove a remote cluster.
func newRemoteClusterSetting(name string, seedHosts []string) esclient.Settings {
	return esclient.Settings{
		PersistentSettings: &esclient.SettingsGroup{
			Cluster: esclient.Cluster{
				RemoteClusters: map[string]esclient.RemoteCluster{
					name: {
						Seeds: seedHosts,
					},
				},
			},
		},
	}
}

// updateRemoteCluster makes a call to an Elasticsearch cluster to apply a persistent setting.
func updateRemoteCluster(esClient esclient.Client, persistentSettings esclient.Settings) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	return esClient.UpdateSettings(ctx, persistentSettings)
}
