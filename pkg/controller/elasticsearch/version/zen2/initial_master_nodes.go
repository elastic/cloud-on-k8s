// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"context"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
)

const (
	// InitialMasterNodesAnnotation is applied on the Elasticsearch resource while a cluster is
	// bootstrapping zen2, and removed when bootstrapping is done.
	initialMasterNodesAnnotation = "elasticsearch.k8s.elastic.co/initial-master-nodes"
)

// SetupInitialMasterNodes sets the `cluster.initial_master_nodes` configuration setting on
// zen2-compatible master nodes from nodeSpecResources if necessary.
// This is only necessary when bootstrapping a new zen2 cluster, or when upgrading a single zen1 master.
// Rolling upgrades from eg. v6 to v7 do not need that setting.
// It ensures `cluster.initial_master_nodes` does not vary over time, when this function gets called multiple times.
func SetupInitialMasterNodes(es esv1.Elasticsearch, k8sClient k8s.Client, nodeSpecResources nodespec.ResourcesList) error {
	// if the cluster is annotated with `cluster.initial_master_nodes` (zen2 bootstrap in progress),
	// make sure we reuse that value since it is not supposed to vary over time
	if initialMasterNodes := getInitialMasterNodesAnnotation(es); initialMasterNodes != nil {
		return patchInitialMasterNodesConfig(nodeSpecResources, initialMasterNodes)
	}

	// in most cases, `cluster.initial_master_nodes` should not be set
	shouldSetup, err := shouldSetInitialMasterNodes(es, k8sClient, nodeSpecResources)
	if err != nil {
		return err
	}
	if !shouldSetup {
		return nil
	}

	initialMasterNodes := nodeSpecResources.MasterNodesNames()
	if len(initialMasterNodes) == 0 {
		return pkgerrors.Errorf("no master node found to compute `cluster.initial_master_nodes`")
	}
	log.Info(
		"Setting `cluster.initial_master_nodes`",
		"namespace", es.Namespace,
		"es_name", es.Name,
		"cluster.initial_master_nodes", strings.Join(initialMasterNodes, ","),
	)
	if err := patchInitialMasterNodesConfig(nodeSpecResources, initialMasterNodes); err != nil {
		return err
	}
	// keep the computed value in an annotation for reuse in subsequent reconciliations
	return setInitialMasterNodesAnnotation(k8sClient, es, initialMasterNodes)
}

func shouldSetInitialMasterNodes(es esv1.Elasticsearch, k8sClient k8s.Client, nodeSpecResources nodespec.ResourcesList) (bool, error) {
	if v, err := version.Parse(es.Spec.Version); err != nil || !versionCompatibleWithZen2(*v) {
		// we only care about zen2-compatible clusters here
		return false, err
	}
	// we want to set `cluster.initial_master_nodes` if:
	// - a new cluster is getting created (not already bootstrapped)
	if !bootstrap.AnnotatedForBootstrap(es) {
		return true, nil
	}
	// - we're upgrading (effectively restarting) a single zen1 master to zen2
	return singleZen1MasterUpgrade(k8sClient, es, nodeSpecResources)
}

// RemoveZen2BootstrapAnnotation removes the initialMasterNodesAnnotation (if set) once zen2 is bootstrapped
// on the corresponding cluster.
func RemoveZen2BootstrapAnnotation(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, esClient client.Client) (bool, error) {
	if v, err := version.Parse(es.Spec.Version); err != nil || !versionCompatibleWithZen2(*v) {
		// we only care about zen2-compatible clusters here
		return false, err
	}
	if getInitialMasterNodesAnnotation(es) == nil {
		// most common case: no annotation set, nothing to do
		return false, nil
	}
	// the cluster was annotated to indicate it is performing a zen2 bootstrap,
	// let's check if that bootstrap is done so we can remove the annotation
	isBootstrapped, err := esClient.ClusterBootstrappedForZen2(ctx)
	if err != nil {
		return false, err
	}
	if !isBootstrapped {
		// retry later
		return true, nil
	}
	log.Info("Zen 2 bootstrap is complete",
		"namespace", es.Namespace,
		"es_name", es.Name,
	)
	// remove the annotation to indicate we're done with zen2 bootstrapping
	delete(es.Annotations, initialMasterNodesAnnotation)
	return false, k8sClient.Update(context.Background(), &es)
}

// patchInitialMasterNodesConfig mutates the configuration of zen2-compatible master nodes
// to have the given `cluster.initial_master_nodes` setting.
func patchInitialMasterNodesConfig(nodeSpecResources nodespec.ResourcesList, initialMasterNodes []string) error {
	for i, res := range nodeSpecResources {
		if !label.IsMasterNodeSet(res.StatefulSet) || !IsCompatibleWithZen2(res.StatefulSet) {
			// we only care about updating zen2 masters config here
			continue
		}
		if err := nodeSpecResources[i].Config.SetStrings(esv1.ClusterInitialMasterNodes, initialMasterNodes...); err != nil {
			return err
		}
	}
	return nil
}

// singleZen1MasterUpgrade returns true if expected nodes in nodeSpecResources will lead to upgrading
// the single zen1-compatible master node currently running in the es cluster.
func singleZen1MasterUpgrade(c k8s.Client, es esv1.Elasticsearch, nodeSpecResources nodespec.ResourcesList) (bool, error) {
	// looking for a single master node...
	masters, err := sset.GetActualMastersForCluster(c, es)
	if err != nil {
		return false, err
	}
	if len(masters) != 1 {
		return false, nil
	}
	currentMaster := masters[0]
	// ...not compatible with zen2...
	v, err := label.ExtractVersion(currentMaster.Labels)
	if err != nil {
		return false, err
	}
	if versionCompatibleWithZen2(*v) {
		return false, nil
	}
	// ...that will be replaced
	var targetMasters []string
	for _, res := range nodeSpecResources {
		if label.IsMasterNodeSet(res.StatefulSet) {
			targetMasters = append(targetMasters, sset.PodNames(res.StatefulSet)...)
		}
	}
	if len(targetMasters) == 0 {
		return false, nil
	}
	if len(targetMasters) > 1 {
		// Covers the case where the user is upgrading to zen2 + adding more masters simultaneously.
		// Additional masters will get created before the existing one gets upgraded/restarted.
		return false, nil
	}
	if targetMasters[0] != currentMaster.Name {
		// Covers the case where the existing master is replaced by another master in a different NodeSet.
		// The new master will be created before the existing one gets removed.
		return false, nil
	}
	// single zen1 master, will be replaced by a single zen2 master with the same name
	return true, nil
}

// getInitialMasterNodesAnnotation parses the `cluster.initial_master_nodes` value from
// annotations on es, or returns nil if not set.
func getInitialMasterNodesAnnotation(es esv1.Elasticsearch) []string {
	var nodes []string
	if value := es.Annotations[initialMasterNodesAnnotation]; value != "" {
		nodes = strings.Split(value, ",")
	}
	return nodes
}

// setInitialMasterNodesAnnotation sets initialMasterNodesAnnotation on the given es resource to initialMasterNodes,
// and updates the es resource in the apiserver.
func setInitialMasterNodesAnnotation(k8sClient k8s.Client, es esv1.Elasticsearch, initialMasterNodes []string) error {
	if es.Annotations == nil {
		es.Annotations = map[string]string{}
	}
	es.Annotations[initialMasterNodesAnnotation] = strings.Join(initialMasterNodes, ",")
	return k8sClient.Update(context.Background(), &es)
}
