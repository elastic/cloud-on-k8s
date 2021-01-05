// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"context"
	"sort"
	"strings"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	// ManagedRemoteClustersAnnotationName holds the list of the remote clusters which have been created
	ManagedRemoteClustersAnnotationName = "elasticsearch.k8s.elastic.co/managed-remote-clusters"
)

// getRemoteClustersInAnnotation returns a set that contains a list of remote clusters that may have been declared in Elasticsearch.
// A map is returned here to quickly compare with the ones that are new or missing.
// If there's no remote clusters the map is empty but not nil.
func getRemoteClustersInAnnotation(es esv1.Elasticsearch) map[string]struct{} {
	remoteClusters := make(map[string]struct{})
	serializedRemoteClusters, ok := es.Annotations[ManagedRemoteClustersAnnotationName]
	if !ok || strings.TrimSpace(serializedRemoteClusters) == "" {
		return remoteClusters
	}
	for _, remoteClusterInAnnotation := range strings.Split(serializedRemoteClusters, ",") {
		remoteClusters[remoteClusterInAnnotation] = struct{}{}
	}
	return remoteClusters
}

func annotateWithCreatedRemoteClusters(c k8s.Client, es esv1.Elasticsearch, remoteClusters map[string]struct{}) error {
	if len(remoteClusters) == 0 {
		// if there are no annotations, there's nothing to do
		if len(es.Annotations) == 0 {
			return nil
		}

		// if the annotation exists, delete it
		if _, ok := es.Annotations[ManagedRemoteClustersAnnotationName]; ok {
			delete(es.Annotations, ManagedRemoteClustersAnnotationName)
			return c.Update(context.Background(), &es)
		}

		return nil
	}

	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}

	annotation := make([]string, 0, len(remoteClusters))
	for remoteCluster := range remoteClusters {
		annotation = append(annotation, remoteCluster)
	}

	sort.Strings(annotation)
	es.Annotations[ManagedRemoteClustersAnnotationName] = strings.Join(annotation, ",")

	return c.Update(context.Background(), &es)
}
