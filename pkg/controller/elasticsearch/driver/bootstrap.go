// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// ClusterUUIDAnnotationName used to store the cluster UUID as an annotation when cluster has been bootstrapped.
	ClusterUUIDAnnotationName = "elasticsearch.k8s.elastic.co/cluster-uuid"
)

// AnnotatedForBootstrap returns true if the cluster has been annotated with the UUID already.
func AnnotatedForBootstrap(cluster v1alpha1.Elasticsearch) bool {
	_, bootstrapped := cluster.Annotations[ClusterUUIDAnnotationName]
	return bootstrapped
}

func ReconcileClusterUUID(c k8s.Client, cluster *v1alpha1.Elasticsearch, observedState observer.State) error {
	if AnnotatedForBootstrap(*cluster) {
		// already annotated, nothing to do.
		return nil
	}
	if clusterIsBootstrapped(observedState) {
		// cluster bootstrapped but not annotated yet
		return annotateWithUUID(cluster, observedState, c)
	}
	// cluster not bootstrapped yet
	return nil
}

// clusterIsBootstrapped returns true if the cluster has formed and has a UUID.
func clusterIsBootstrapped(observedState observer.State) bool {
	return observedState.ClusterState != nil && len(observedState.ClusterState.ClusterUUID) > 0
}

// annotateWithUUID annotates the cluster with its UUID, to mark it as "bootstrapped".
func annotateWithUUID(cluster *v1alpha1.Elasticsearch, observedState observer.State, c k8s.Client) error {
	log.Info("Annotating bootstrapped cluster with its UUID", "namespace", cluster.Namespace, "es_name", cluster.Name)
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	cluster.Annotations[ClusterUUIDAnnotationName] = observedState.ClusterState.ClusterUUID
	return c.Update(cluster)
}
