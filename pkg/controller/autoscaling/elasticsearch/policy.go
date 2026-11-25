// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/go-logr/logr"
	"go.elastic.co/apm/v2"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// updatePolicies updates the autoscaling policies in the Elasticsearch cluster.
func updatePolicies(
	ctx context.Context,
	log logr.Logger,
	autoscalingPolicySpecs v1alpha1.AutoscalingPolicySpecs,
	esclient client.AutoscalingClient,
) error {
	span, _ := apm.StartSpan(ctx, "update_autoscaling_policies", tracing.SpanTypeApp)
	defer span.End()
	// Update policies in place. Deleting them all first clears Elasticsearch's cached node information,
	// so the capacity request that follows can race with the asynchronous refresh and return no recommendation.
	// Unchanged PUTs do not refresh that cache; retrying failed node-information fetches needs a fix in Elasticsearch.
	for _, rp := range autoscalingPolicySpecs {
		if err := esclient.CreateAutoscalingPolicy(ctx, rp.Name, rp.AutoscalingPolicy); err != nil {
			log.Error(err, "Error while updating an autoscaling policy", "policy", rp.Name)
			return err
		}
	}
	return nil
}

// deleteObsoletePolicies deletes the policies that exist in Elasticsearch but not in the autoscaler spec.
// It finds them in the capacity response, which lists every policy of the cluster.
// Policies created outside ECK are also deleted if absent from the autoscaler spec.
// Desired policies have already been created or updated, so their nodes stay covered even during a policy rename.
// Failed deletes are collected while cleanup continues. The next reconciliation retries the remaining policies,
// without discarding valid recommendations.
func deleteObsoletePolicies(
	ctx context.Context,
	log logr.Logger,
	autoscalingPolicySpecs v1alpha1.AutoscalingPolicySpecs,
	capacity client.AutoscalingCapacityResult,
	esclient client.AutoscalingClient,
) error {
	desiredPolicies := set.Make()
	for _, policy := range autoscalingPolicySpecs {
		desiredPolicies.Add(policy.Name)
	}
	var errs []error
	// Sort the names so that the deletes and the reported errors keep the same order between reconciliations.
	for _, policyName := range slices.Sorted(maps.Keys(capacity.Policies)) {
		if desiredPolicies.Has(policyName) {
			continue
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := esclient.DeleteAutoscalingPolicy(ctx, policyName); err != nil {
			if client.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("delete obsolete autoscaling policy %q: %w", policyName, err))
			continue
		}
		log.Info("Deleted obsolete autoscaling policy", "policy", policyName)
	}
	return errors.Join(errs...)
}
