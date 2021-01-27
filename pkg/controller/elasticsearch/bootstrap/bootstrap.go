// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bootstrap

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"go.elastic.co/apm"
)

var log = ulog.Log.WithName("elasticsearch-uuid")

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
func ReconcileClusterUUID(ctx context.Context, k8sClient k8s.Client, cluster *esv1.Elasticsearch, esClient client.Client, esReachable bool) (bool, error) {
	span, ctx := apm.StartSpan(ctx, "reconcile_cluster_uuid", tracing.SpanTypeApp)
	defer span.End()

	if AnnotatedForBootstrap(*cluster) {
		// already annotated, nothing to do.
		return false, nil
	}
	if !esReachable {
		// retry later
		return true, nil
	}
	clusterUUID, err := getClusterUUID(ctx, esClient)
	if err != nil {
		// There was an error while retrieving the UUID of the Elasticsearch cluster.
		// For example, it could be the case with ES 6.x if the cluster does not have a master yet, in this case an
		// API call to get the cluster UUID returns a 503 error.
		// However we don't want to stop the reconciliation loop here because it could prevent the user to apply
		// an update to the cluster spec to fix a problem.
		// Therefore we just log the error and notify the driver that the reconciliation should be eventually re-queued.
		log.Info(
			"Recoverable error while retrieving Elasticsearch cluster UUID",
			"namespace", cluster.Namespace,
			"es_name", cluster.Name,
			"error", err,
		)
		return true, nil
	}
	if !isUUIDValid(clusterUUID) {
		// retry later
		return true, nil
	}
	return false, annotateWithUUID(k8sClient, cluster, clusterUUID)
}

// getClusterUUID retrieves the cluster UUID using the given esClient.
func getClusterUUID(ctx context.Context, esClient client.Client) (string, error) {
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
	return k8sClient.Update(context.Background(), cluster)
}
