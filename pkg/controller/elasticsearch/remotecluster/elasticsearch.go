// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"context"
	"sort"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

var log = ulog.Log.WithName("remotecluster")

const enterpriseFeaturesDisabledMsg = "Remote cluster is an enterprise feature. Enterprise features are disabled"

// UpdateSettings updates the remote clusters in the persistent settings by calling the Elasticsearch API.
// A boolean is returned to indicate if a requeue should be scheduled to sync the annotation on the Elasticsearch object
// when the remote clusters that are not expected anymore are actually deleted from the Elasticsearch settings.
// See the documentation of updateSettingsInternal for more information about the algorithm.
func UpdateSettings(
	ctx context.Context,
	c k8s.Client,
	esClient esclient.Client,
	eventRecorder record.EventRecorder,
	licenseChecker license.Checker,
	es esv1.Elasticsearch,
) (bool, error) {
	span, _ := apm.StartSpan(ctx, "update_remote_clusters", tracing.SpanTypeApp)
	defer span.End()

	remoteClustersInSpec := getRemoteClustersInSpec(es)
	enabled, err := licenseChecker.EnterpriseFeaturesEnabled()
	if err != nil {
		return true, err
	}
	if !enabled && len(remoteClustersInSpec) > 0 {
		log.Info(
			enterpriseFeaturesDisabledMsg,
			"namespace", es.Namespace, "es_name", es.Name,
		)
		eventRecorder.Eventf(&es, corev1.EventTypeWarning, events.EventAssociationError, enterpriseFeaturesDisabledMsg)
		return false, nil
	}

	return updateSettingsInternal(remoteClustersInSpec, c, esClient, es)
}

// updateSettingsInternal updates remote clusters in Elasticsearch. It also keeps track of any remote clusters which
// have been declared in the Elasticsearch spec. The purpose is to delete remote clusters which were managed by
// the operator but are not desired anymore, without removing the ones which have been added by the user.
// The following algorithm is used:
// 1. Get the list of the previously declared remote clusters from the annotation
// 2. Ensure that all remote clusters in the Elasticsearch spec are present in the annotation
// 3. For each remote cluster in the annotation which is not in the Spec, either:
//   3.1 Schedule its deletion from the Elasticsearch settings
//   3.2 Otherwise remove it from the annotation
// 4. Update the annotation on the Elasticsearch object
// 5. Apply the settings through the Elasticsearch API
func updateSettingsInternal(
	remoteClustersInSpec map[string]esv1.RemoteCluster,
	c k8s.Client,
	esClient esclient.Client,
	es esv1.Elasticsearch,
) (requeue bool, err error) {
	remoteClustersInAnnotation := getRemoteClustersInAnnotation(es)

	// Retrieve the remote clusters currently declared in Elasticsearch
	remoteClustersInEs, err := getRemoteClustersInElasticsearch(esClient)
	if err != nil {
		return true, err
	}

	var remoteClustersToDelete []string
	// For each remote cluster in the annotation but not in the spec, either:
	// * Schedule its deletion if it exists in the Elasticsearch settings
	// * Remove it from the annotation if it does not exist anymore in Elasticsearch settings
	for remoteClusterInAnnotation := range remoteClustersInAnnotation {
		if _, inSpec := remoteClustersInSpec[remoteClusterInAnnotation]; inSpec {
			continue
		}
		_, inElasticsearch := remoteClustersInEs[remoteClusterInAnnotation]
		if inElasticsearch {
			// This remote cluster is in the annotation and in Elasticsearch but not in the Spec: we should delete it
			remoteClustersToDelete = append(remoteClustersToDelete, remoteClusterInAnnotation)
		} else {
			// This remote cluster in the annotation is neither in the Spec or in Elasticsearch, we don't need to track it anymore
			delete(remoteClustersInAnnotation, remoteClusterInAnnotation)
		}
	}

	remoteClustersToUpdate := make([]string, 0, len(remoteClustersInSpec)) // only used for logging
	// remoteClustersToApply are clusters to add (or update) based on what is specified in the Elasticsearch spec.
	remoteClustersToApply := make(map[string]esclient.RemoteCluster)
	for name, remoteCluster := range remoteClustersInSpec {
		remoteClustersToUpdate = append(remoteClustersToUpdate, name)
		// Declare remote cluster in ES
		seedHosts := []string{services.ExternalTransportServiceHost(remoteCluster.ElasticsearchRef.NamespacedName())}
		remoteClustersToApply[name] = esclient.RemoteCluster{Seeds: seedHosts}
		// Ensure this cluster is tracked in the annotation
		remoteClustersInAnnotation[name] = struct{}{}
	}

	// RemoteClusters to remove from Elasticsearch
	for _, name := range remoteClustersToDelete {
		remoteClustersToApply[name] = esclient.RemoteCluster{Seeds: nil}
	}

	// Update the annotation
	if err := annotateWithCreatedRemoteClusters(c, es, remoteClustersInAnnotation); err != nil {
		return true, err
	}

	// Since the annotation is updated before Elasticsearch we should requeue to sync the annotation
	// if some clusters are deleted from Elasticsearch.
	requeue = len(remoteClustersToDelete) > 0
	if len(remoteClustersToApply) > 0 {
		// Apply the settings
		sort.Strings(remoteClustersToUpdate)
		sort.Strings(remoteClustersToDelete)
		log.Info("Updating remote cluster settings",
			"namespace", es.Namespace,
			"es_name", es.Name,
			"updated_remote_clusters", remoteClustersToUpdate,
			"deleted_remote_clusters", remoteClustersToDelete,
		)
		return requeue, updateSettings(esClient, remoteClustersToApply)
	}
	return requeue, nil
}

// getRemoteClustersInElasticsearch returns all the remote clusters currently declared in Elasticsearch
func getRemoteClustersInElasticsearch(esClient esclient.Client) (map[string]struct{}, error) {
	remoteClustersInEs := make(map[string]struct{})
	remoteClusterSettings, err := esClient.GetRemoteClusterSettings(context.Background())
	if err != nil {
		return remoteClustersInEs, err
	}
	for remoteClusterName := range remoteClusterSettings.PersistentSettings.Cluster.RemoteClusters {
		remoteClustersInEs[remoteClusterName] = struct{}{}
	}
	return remoteClustersInEs, nil
}

// getRemoteClustersInSpec returns a map with the expected remote clusters as declared by the user in the Elasticsearch specification.
// A map is returned here because it will be used to quickly compare with the ones that are new or missing.
func getRemoteClustersInSpec(es esv1.Elasticsearch) map[string]esv1.RemoteCluster {
	remoteClusters := make(map[string]esv1.RemoteCluster)
	for _, remoteCluster := range es.Spec.RemoteClusters {
		if !remoteCluster.ElasticsearchRef.IsDefined() {
			continue
		}
		remoteCluster.ElasticsearchRef = remoteCluster.ElasticsearchRef.WithDefaultNamespace(es.Namespace)
		remoteClusters[remoteCluster.Name] = remoteCluster
	}
	return remoteClusters
}

// updateSettings makes a call to an Elasticsearch cluster to apply a persistent setting.
func updateSettings(esClient esclient.Client, remoteClusters map[string]esclient.RemoteCluster) error {
	return esClient.UpdateRemoteClusterSettings(context.Background(), esclient.RemoteClustersSettings{
		PersistentSettings: &esclient.SettingsGroup{
			Cluster: esclient.RemoteClusters{
				RemoteClusters: remoteClusters,
			},
		},
	})
}
