// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	commonvolume "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func RecreateStatefulSets(ctx context.Context, k8sclient k8s.Client, ls logstashv1alpha1.Logstash) (int, error) {
	return commonvolume.RecreateStatefulSets(ctx, k8sclient, &ls, ls.Kind)
}

func HandleVolumeExpansion(ctx context.Context, k8sClient k8s.Client, ls logstashv1alpha1.Logstash,
	expectedSset appsv1.StatefulSet, actualSset appsv1.StatefulSet,
	validateStorageClass bool) (bool, error) {
	return commonvolume.HandleVolumeExpansion(ctx, k8sClient, &ls, ls.Kind, expectedSset,
		actualSset, validateStorageClass)
}
