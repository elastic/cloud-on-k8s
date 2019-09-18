// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package zen2

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
)

// SetupInitialMasterNodes modifies the ES config of the given resources to setup
// cluster initial master nodes.
func SetupInitialMasterNodes(
	nodeSpecResources nodespec.ResourcesList,
) error {
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
