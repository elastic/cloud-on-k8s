// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version/zen2"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// ClusterUUIDAnnotationName used to store the cluster UUID as an annotation when cluster has been bootstrapped.
	ClusterUUIDAnnotationName = "elasticsearch.k8s.elastic.co/cluster-uuid"
	formingClusterUUID        = "_na_"
)

// AnnotatedForBootstrap returns true if the cluster has been annotated with the UUID already.
func AnnotatedForBootstrap(cluster v1beta1.Elasticsearch) bool {
	_, bootstrapped := cluster.Annotations[ClusterUUIDAnnotationName]
	return bootstrapped
}

func ReconcileClusterUUID(c k8s.Client, cluster *v1beta1.Elasticsearch, observedState observer.State) error {
	reBootstrap, err := clusterNeedsReBootstrap(c, cluster)
	if err != nil {
		return err
	}

	if AnnotatedForBootstrap(*cluster) {
		if reBootstrap {
			log.Info("cluster re-bootstrap necessary",
				"version", cluster.Spec.Version,
				"namespace", cluster.Namespace,
				"name", cluster.Name,
			)
			return removeUUIDAnnotation(c, cluster)
		}
		// already annotated, nothing to do.
		return nil
	}
	if clusterIsBootstrapped(observedState) && !reBootstrap {
		// cluster bootstrapped but not annotated yet
		return annotateWithUUID(cluster, observedState, c)
	}
	// cluster not bootstrapped yet
	return nil
}

func removeUUIDAnnotation(client k8s.Client, es *v1beta1.Elasticsearch) error {
	annotations := es.Annotations
	if annotations == nil {
		return nil
	}
	delete(es.Annotations, ClusterUUIDAnnotationName)
	return client.Update(es)
}

// clusterNeedsReBootstrap is true if we are updating a single master cluster from 6.x to 7.x
// because we lose the 'cluster' when rolling the single master node.
// Invariant: no grow and shrink
func clusterNeedsReBootstrap(client k8s.Client, es *v1beta1.Elasticsearch) (bool, error) {
	initialZen2Upgrade, err := zen2.IsInitialZen2Upgrade(client, *es)
	if err != nil {
		return false, err
	}
	currentMasters, err := sset.GetActualMastersForCluster(client, *es)
	if err != nil {
		return false, err
	}
	return len(currentMasters) == 1 && initialZen2Upgrade, nil
}

// clusterIsBootstrapped returns true if the cluster has formed and has a UUID.
func clusterIsBootstrapped(observedState observer.State) bool {
	return observedState.ClusterInfo != nil &&
		len(observedState.ClusterInfo.ClusterUUID) > 0 &&
		observedState.ClusterInfo.ClusterUUID != formingClusterUUID
}

// annotateWithUUID annotates the cluster with its UUID, to mark it as "bootstrapped".
func annotateWithUUID(cluster *v1beta1.Elasticsearch, observedState observer.State, c k8s.Client) error {
	log.Info("Annotating bootstrapped cluster with its UUID", "namespace", cluster.Namespace, "es_name", cluster.Name)
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	cluster.Annotations[ClusterUUIDAnnotationName] = observedState.ClusterInfo.ClusterUUID
	return c.Update(cluster)
}
