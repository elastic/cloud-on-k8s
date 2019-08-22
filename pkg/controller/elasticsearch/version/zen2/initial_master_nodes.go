// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// ClusterUUIDAnnotationName used to store the cluster UUID as an annotation when cluster has been bootstrapped.
	ClusterUUIDAnnotationName = "elasticsearch.k8s.elastic.co/cluster-uuid"
)

// annotatedForBootstrap returns true if the cluster has been annotated with the UUID already.
func annotatedForBootstrap(cluster v1alpha1.Elasticsearch) bool {
	_, bootstrapped := cluster.Annotations[ClusterUUIDAnnotationName]
	return bootstrapped
}

// clusterIsBootstrapped returns true if the cluster has formed and has a UUID.
func clusterIsBootstrapped(observedState observer.State) bool {
	return observedState.ClusterState != nil && len(observedState.ClusterState.ClusterUUID) > 0
}

// annotateWithUUID annotates the cluster with its UUID, to mark it as "bootstrapped".
func annotateWithUUID(cluster v1alpha1.Elasticsearch, observedState observer.State, c k8s.Client) error {
	log.Info("Annotating bootstrapped cluster with its UUID", "namespace", cluster.Namespace, "es_name", cluster.Name)
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	cluster.Annotations[ClusterUUIDAnnotationName] = observedState.ClusterState.ClusterUUID
	if err := c.Update(&cluster); err != nil {
		return err
	}
	return nil
}

// SetupInitialMasterNodes modifies the ES config of the given resources to setup
// cluster initial master nodes.
// It also saves the cluster UUID as an annotation to ensure that it's not set
// if the cluster has already been bootstrapped.
func SetupInitialMasterNodes(
	cluster v1alpha1.Elasticsearch,
	observedState observer.State,
	c k8s.Client,
	nodeSpecResources nodespec.ResourcesList,
) error {
	if annotatedForBootstrap(cluster) {
		// Cluster already bootstrapped, nothing to do.
		return nil
	}

	if clusterIsBootstrapped(observedState) {
		// Cluster is not annotated for bootstrap, but should be.
		if err := annotateWithUUID(cluster, observedState, c); err != nil {
			return err
		}
		return nil
	}

	// Cluster is not bootstrapped yet, set initial_master_nodes setting in each master node config.
	masters := nodeSpecResources.MasterNodesNames()
	if len(masters) == 0 {
		return nil
	}
	for i, res := range nodeSpecResources {
		if !IsCompatibleWithZen2(res.StatefulSet) {
			continue
		}
		if !label.IsMasterNodeSet(res.StatefulSet) {
			// we only care about master nodes config here
			continue
		}
		// patch config with the expected initial master nodes
		if err := nodeSpecResources[i].Config.SetStrings(settings.ClusterInitialMasterNodes, masters...); err != nil {
			return err
		}
	}
	return nil
}
