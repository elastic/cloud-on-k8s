// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package zen2

import (
	"context"
	"strings"

	pkgerrors "github.com/pkg/errors"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	// InitialMasterNodesAnnotation is applied on the Elasticsearch resource while a cluster is
	// bootstrapping zen2, and removed when bootstrapping is done.
	InitialMasterNodesAnnotation = "elasticsearch.k8s.elastic.co/initial-master-nodes"
)

// SetupInitialMasterNodes sets the `cluster.initial_master_nodes` configuration setting on
// zen2-compatible master nodes from nodeSpecResources if necessary.
// This is only necessary when bootstrapping a new zen2 cluster.
// It ensures `cluster.initial_master_nodes` does not vary over time, when this function gets called multiple times.
func SetupInitialMasterNodes(ctx context.Context, es esv1.Elasticsearch, k8sClient k8s.Client, nodeSpecResources nodespec.ResourcesList) error {
	// if the cluster is annotated with `cluster.initial_master_nodes` (zen2 bootstrap in progress),
	// make sure we reuse that value since it is not supposed to vary over time
	if initialMasterNodes := getInitialMasterNodesAnnotation(es); initialMasterNodes != nil {
		return patchInitialMasterNodesConfig(ctx, nodeSpecResources, initialMasterNodes)
	}

	// in most cases, `cluster.initial_master_nodes` should not be set
	shouldSetup, err := shouldSetInitialMasterNodes(es)
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
	ulog.FromContext(ctx).Info(
		"Setting `cluster.initial_master_nodes`",
		"namespace", es.Namespace,
		"es_name", es.Name,
		"cluster.initial_master_nodes", strings.Join(initialMasterNodes, ","),
	)
	if err := patchInitialMasterNodesConfig(ctx, nodeSpecResources, initialMasterNodes); err != nil {
		return err
	}
	// keep the computed value in an annotation for reuse in subsequent reconciliations
	return setInitialMasterNodesAnnotation(ctx, k8sClient, es, initialMasterNodes)
}

func shouldSetInitialMasterNodes(es esv1.Elasticsearch) (bool, error) {
	if v, err := version.Parse(es.Spec.Version); err != nil || !versionCompatibleWithZen2(v) {
		// we only care about zen2-compatible clusters here
		return false, err
	}
	// Set cluster.initial_master_nodes only when a new cluster is getting created (not already bootstrapped)
	return !bootstrap.AnnotatedForBootstrap(es), nil
}

// RemoveZen2BootstrapAnnotation removes the initialMasterNodesAnnotation (if set) once zen2 is bootstrapped
// on the corresponding cluster.
func RemoveZen2BootstrapAnnotation(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, esClient client.Client) (bool, error) {
	if v, err := version.Parse(es.Spec.Version); err != nil || !versionCompatibleWithZen2(v) {
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
	ulog.FromContext(ctx).Info("Zen 2 bootstrap is complete",
		"namespace", es.Namespace,
		"es_name", es.Name,
	)
	// remove the annotation to indicate we're done with zen2 bootstrapping
	delete(es.Annotations, InitialMasterNodesAnnotation)
	return false, k8sClient.Update(ctx, &es)
}

// patchInitialMasterNodesConfig mutates the configuration of zen2-compatible master nodes
// to have the given `cluster.initial_master_nodes` setting.
func patchInitialMasterNodesConfig(ctx context.Context, nodeSpecResources nodespec.ResourcesList, initialMasterNodes []string) error {
	for i, res := range nodeSpecResources {
		if !label.IsMasterNodeSet(res.StatefulSet) || !IsCompatibleWithZen2(ctx, res.StatefulSet) {
			// we only care about updating zen2 masters config here
			continue
		}
		if err := nodeSpecResources[i].Config.SetStrings(esv1.ClusterInitialMasterNodes, initialMasterNodes...); err != nil {
			return err
		}
	}
	return nil
}

// getInitialMasterNodesAnnotation parses the `cluster.initial_master_nodes` value from
// annotations on es, or returns nil if not set.
func getInitialMasterNodesAnnotation(es esv1.Elasticsearch) []string {
	var nodes []string
	if value := es.Annotations[InitialMasterNodesAnnotation]; value != "" {
		nodes = strings.Split(value, ",")
	}
	return nodes
}

// setInitialMasterNodesAnnotation sets initialMasterNodesAnnotation on the given es resource to initialMasterNodes,
// and updates the es resource in the apiserver.
func setInitialMasterNodesAnnotation(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, initialMasterNodes []string) error {
	if es.Annotations == nil {
		es.Annotations = map[string]string{}
	}
	es.Annotations[InitialMasterNodesAnnotation] = strings.Join(initialMasterNodes, ",")
	return k8sClient.Update(ctx, &es)
}
