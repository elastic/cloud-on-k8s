// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bootstrap

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"

	"context"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("elasticsearch-uuid")

const (
	// ClusterUUIDAnnotationName used to store the cluster UUID as an annotation when cluster has been bootstrapped.
	ClusterUUIDAnnotationName = "elasticsearch.k8s.elastic.co/cluster-uuid"
	formingClusterUUID        = "_na_"
)

// AnnotatedForBootstrap returns true if the cluster has been annotated with the UUID already.
func AnnotatedForBootstrap(cluster esv1.Elasticsearch) bool {
	_, bootstrapped := cluster.Annotations[ClusterUUIDAnnotationName]
	return bootstrapped
}

// ReconcileClusterUUID attempts to set the ClusterUUID annotation on the Elasticsearch resource if not already set.
// It returns a boolean indicating whether the reconciliation should be re-queued (ES not reachable).
func ReconcileClusterUUID(k8sClient k8s.Client, cluster *esv1.Elasticsearch, esClient client.Client, esReachable bool) (bool, error) {
	if AnnotatedForBootstrap(*cluster) {
		// already annotated, nothing to do.
		return false, nil
	}
	if !esReachable {
		// retry later
		return true, nil
	}
	clusterUUID, err := getClusterUUID(esClient)
	if err != nil {
		return false, err
	}
	if !isUUIDValid(clusterUUID) {
		// retry later
		return true, nil
	}
	return false, annotateWithUUID(k8sClient, cluster, clusterUUID)
}

// getClusterUUID retrieves the cluster UUID using the given esClient.
func getClusterUUID(esClient client.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
	defer cancel()
	info, err := esClient.GetClusterInfo(ctx)
	if err != nil {
		return "", err
	}
	return info.ClusterUUID, nil
}

// isUUIDValid returns true if the uuid corresponds to formed cluster UUID.
func isUUIDValid(uuid string) bool {
	return uuid != "" && uuid != formingClusterUUID
}

// annotateWithUUID annotates the cluster with its UUID, to mark it as "bootstrapped".
func annotateWithUUID(k8sClient k8s.Client, cluster *esv1.Elasticsearch, uuid string) error {
	log.Info(
		"Annotating bootstrapped cluster with its UUID",
		"namespace", cluster.Namespace,
		"es_name", cluster.Name,
		"uuid", uuid,
	)
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	cluster.Annotations[ClusterUUIDAnnotationName] = uuid
	return k8sClient.Update(cluster)
}
