// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateless

import (
	"context"
	"fmt"
	"sort"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

func (sd *statelessDriver) expectationsSatisfied(ctx context.Context) (bool, string, error) {
	// check if actual CloneSets match our expectations before applying any change
	ok, reason, err := sd.Expectations.Satisfied()
	if err != nil {
		return false, reason, err
	}
	if !ok {
		ulog.FromContext(ctx).Info("Cache expectations are not satisfied yet, re-queueing", "namespace", sd.ES.Namespace, "es_name", sd.ES.Name, "reason", reason)
		return false, reason, nil
	}

	// check if all CloneSets most recent generation is observed before applying any change.
	cloneSets := v1alpha1.CloneSetList{}
	if err := sd.Client.List(ctx, &cloneSets, client.InNamespace(sd.ES.Namespace), label.NewLabelSelectorForElasticsearchClusterName(sd.ES.Name)); err != nil {
		return false, "", err
	}
	// sort CloneSets by name to have a stable returned result
	sort.Slice(cloneSets.Items, func(i, j int) bool {
		return cloneSets.Items[i].Name < cloneSets.Items[j].Name
	})
	for _, cs := range cloneSets.Items {
		if cs.Generation != cs.Status.ObservedGeneration {
			ulog.FromContext(ctx).Info("Waiting for CloneSet to be observed before applying further changes", "cloneSet", k8s.ExtractNamespacedName(&cs))
			return false, fmt.Sprintf("Waiting for CloneSet %s/%s generation %d to be observed", cs.Namespace, cs.Name, cs.Generation), nil
		}
	}
	return true, "", nil
}
