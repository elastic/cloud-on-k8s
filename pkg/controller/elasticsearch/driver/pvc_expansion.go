// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func recreateStatefulSets(ctx context.Context, k8sclient k8s.Client, es esv1.Elasticsearch) (int, error) {
	return volume.RecreateStatefulSets(ctx, k8sclient, &es, es.Kind)
}

func handleVolumeExpansion(ctx context.Context, k8sClient k8s.Client, es esv1.Elasticsearch, expectedSset appsv1.StatefulSet,
	actualSset appsv1.StatefulSet, validateStorageClass bool) (bool, error) {
	return volume.HandleVolumeExpansion(ctx, k8sClient, &es, es.Kind, expectedSset, actualSset,
		validateStorageClass)
}
