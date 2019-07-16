// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
)

// SetupInitialMasterNodes modifies the ES config of the given resources to setup
// cluster initial master nodes.
func SetupInitialMasterNodes(nodeSpecResources nodespec.ResourcesList) error {
	// TODO: handle zen2 initial master nodes more cleanly
	//  should be empty once cluster is bootstraped
	// TODO: see https://github.com/elastic/cloud-on-k8s/issues/1201 to rely on an annotation
	//  set in the cluster
	masters := nodeSpecResources.MasterNodesNames()
	if len(masters) == 0 {
		return nil
	}
	for i, res := range nodeSpecResources {
		if !IsCompatibleForZen2(res.StatefulSet) {
			continue
		}
		// patch config with the expected initial master nodes
		if err := nodeSpecResources[i].Config.SetStrings(settings.ClusterInitialMasterNodes, masters...); err != nil {
			return err
		}
	}
	return nil
}
