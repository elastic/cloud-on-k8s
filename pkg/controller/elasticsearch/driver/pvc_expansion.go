// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	// RecreateStatefulSetAnnotationPrefix is used to annotate the Elasticsearch resource
	// with StatefulSets to recreate. The StatefulSet name is appended to this name.
	RecreateStatefulSetAnnotationPrefix = "elasticsearch.k8s.elastic.co/recreate-"
)

func recreateStatefulSets(ctx context.Context, k8sclient k8s.Client, es esv1.Elasticsearch) (int, error) {
	return volume.RecreateStatefulSets(ctx, k8sclient, &es, RecreateStatefulSetAnnotationPrefix)
}

