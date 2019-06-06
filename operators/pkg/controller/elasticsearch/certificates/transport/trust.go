// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadTrustRelationships loads the trust relationships from the API.
func LoadTrustRelationships(c k8s.Client, clusterName, namespace string) ([]v1alpha1.TrustRelationship, error) {
	var trs v1alpha1.TrustRelationshipList
	if err := c.List(&client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{label.ClusterNameLabelName: clusterName}),
		Namespace:     namespace,
	}, &trs); err != nil {
		return nil, err
	}

	log.Info("Loaded trust relationships", "clusterName", clusterName, "count", len(trs.Items))

	return trs.Items, nil
}
