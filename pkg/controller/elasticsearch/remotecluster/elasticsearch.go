// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("remotecluster")

const enterpriseFeaturesDisabledMsg = "Remote cluster is an enterprise feature. Enterprise features are disabled"

// UpdateSettings updates the remote clusters in the persistent settings by calling the Elasticsearch API.
func UpdateSettings(
	ctx context.Context,
	c k8s.Client,
	esClient esclient.Client,
	eventRecorder record.EventRecorder,
	licenseChecker license.Checker,
	es esv1.Elasticsearch,
) error {
	span, _ := apm.StartSpan(ctx, "update_remote_clusters", tracing.SpanTypeApp)
	defer span.End()

	expectedRemoteClusters := getExpectedRemoteClusters(es)
	enabled, err := licenseChecker.EnterpriseFeaturesEnabled()
	if err != nil {
		return err
	}
	if !enabled && len(expectedRemoteClusters) > 0 {
		log.Info(
			enterpriseFeaturesDisabledMsg,
			"namespace", es.Namespace, "es_name", es.Name,
		)
		eventRecorder.Eventf(&es, corev1.EventTypeWarning, events.EventAssociationError, enterpriseFeaturesDisabledMsg)
		return nil
	}

	currentRemoteClusters, err := getCurrentRemoteClusters(es)
	if err != nil {
		return err
	}

	remoteClusters := make(map[string]esclient.RemoteCluster)
	// RemoteClusters to add or update
	for name, remoteCluster := range expectedRemoteClusters {
		if currentConfigHash, ok := currentRemoteClusters[name]; !ok || currentConfigHash != remoteCluster.ConfigHash {
			// Declare remote cluster in ES
			seedHosts := []string{services.ExternalTransportServiceHost(remoteCluster.ElasticsearchRef.NamespacedName())}
			log.Info("Adding or updating remote cluster",
				"namespace", es.Namespace,
				"es_name", es.Name,
				"remote_cluster", remoteCluster.Name,
				"seeds", seedHosts,
			)
			remoteClusters[name] = esclient.RemoteCluster{Seeds: seedHosts}
		}
	}

	// RemoteClusters to remove
	for name := range currentRemoteClusters {
		if _, ok := expectedRemoteClusters[name]; !ok {
			log.Info("Removing remote cluster",
				"namespace", es.Namespace,
				"es_name", es.Name,
				"remote_cluster", name,
			)
			remoteClusters[name] = esclient.RemoteCluster{Seeds: nil}
		}
	}

	if len(remoteClusters) > 0 {
		// Apply the settings
		if err := updateSettings(esClient, remoteClusters); err != nil {
			return err
		}
		// Update the annotation
		return annotateWithRemoteClusters(c, es, expectedRemoteClusters)
	}
	return nil
}

// getExpectedRemoteClusters returns a map with the expected remote clusters
// A map is returned here because it will be used to quickly compare with the ones that are new or missing.
func getExpectedRemoteClusters(es esv1.Elasticsearch) map[string]expectedRemoteClusterConfiguration {
	remoteClusters := make(map[string]expectedRemoteClusterConfiguration)
	for _, remoteCluster := range es.Spec.RemoteClusters {
		if !remoteCluster.ElasticsearchRef.IsDefined() {
			continue
		}
		remoteCluster.ElasticsearchRef = remoteCluster.ElasticsearchRef.WithDefaultNamespace(es.Namespace)
		remoteClusters[remoteCluster.Name] = expectedRemoteClusterConfiguration{
			RemoteCluster: remoteCluster,
			ConfigHash:    remoteCluster.ConfigHash(),
		}
	}
	return remoteClusters
}

// updateSettings makes a call to an Elasticsearch cluster to apply a persistent setting.
func updateSettings(esClient esclient.Client, remoteClusters map[string]esclient.RemoteCluster) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	return esClient.UpdateRemoteClusterSettings(ctx, esclient.RemoteClustersSettings{
		PersistentSettings: &esclient.SettingsGroup{
			Cluster: esclient.RemoteClusters{
				RemoteClusters: remoteClusters,
			},
		},
	})
}
