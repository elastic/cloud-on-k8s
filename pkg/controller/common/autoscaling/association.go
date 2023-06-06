// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaling

import (
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esav1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	autoscalerWithDeprecatedAnnotation = "cluster has both the autoscaling annotation and an autoscaler resource associate, please remove the elasticsearch.alpha.elastic.co/autoscaling-* annotations"
	deprecatedAnnotation               = "the autoscaling annotation has been deprecated in favor of the ElasticsearchAutoscaler custom resource"
)

func GetAssociatedAutoscalingResource(
	ctx context.Context,
	k8s k8s.Client,
	es esv1.Elasticsearch,
	recorder record.EventRecorder,
) (v1alpha1.AutoscalingResource, error) {
	// Let's try to detect any associated autoscaler
	autoscalers := &esav1alpha1.ElasticsearchAutoscalerList{}
	if err := k8s.List(ctx, autoscalers, client.InNamespace(es.Namespace)); err != nil {
		return nil, err
	}

	var autoscalingResource v1alpha1.AutoscalingResource
	for _, autoscaler := range autoscalers.Items {
		if autoscaler.Spec.ElasticsearchRef.Name == es.Name {
			autoscalingResource = autoscaler.DeepCopy()
		}
	}

	return autoscalingResource, nil
}
