// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"

	"github.com/go-logr/logr"
	"go.elastic.co/apm/v2"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
)

// updatePolicies updates the autoscaling policies in the Elasticsearch cluster.
func updatePolicies(
	ctx context.Context,
	log logr.Logger,
	autoscalingResource v1alpha1.AutoscalingResource,
	esclient client.AutoscalingClient,
) error {
	span, _ := apm.StartSpan(ctx, "update_autoscaling_policies", tracing.SpanTypeApp)
	defer span.End()
	// Cleanup existing autoscaling policies
	if err := esclient.DeleteAutoscalingPolicies(ctx); err != nil {
		log.Error(err, "Error while deleting policies")
		return err
	}
	autoscalingPolicySpecs, err := autoscalingResource.GetAutoscalingPolicySpecs()
	if err != nil {
		return err
	}
	// Create the expected autoscaling policies
	for _, rp := range autoscalingPolicySpecs {
		if err := esclient.CreateAutoscalingPolicy(ctx, rp.Name, rp.AutoscalingPolicy); err != nil {
			log.Error(err, "Error while updating an autoscaling policy", "policy", rp.Name)
			return err
		}
	}
	return nil
}
