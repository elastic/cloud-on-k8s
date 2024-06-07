// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func newNodeShutdownWatcher(es esv1.Elasticsearch) test.Watcher {
	var observedErrors []error
	maxConcurrentRestarts := int(*es.Spec.UpdateStrategy.ChangeBudget.GetMaxUnavailableOrDefault())
	return test.NewConditionalWatcher("watch for correct node shutdown API usage",
		1*time.Second,
		func(k *test.K8sClient, t *testing.T) { //nolint:thelper
			client, err := NewElasticsearchClient(es, k)
			if err != nil {
				fmt.Printf("error while creating the Elasticsearch client: %s", err)
				if !errors.As(err, &PotentialNetworkError) {
					// explicit API server error, consider as a failure
					observedErrors = append(observedErrors, err)
				}
				return
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), continuousHealthCheckTimeout)
			defer cancel()
			shutdowns, err := client.GetShutdown(ctx, nil)
			if err != nil {
				// we allow errors here, we are not testing for ES availability
				fmt.Printf("error while getting node shutdowns: %s", err)
				return
			}
			var restarts []esclient.NodeShutdown
			for _, s := range shutdowns.Nodes {
				if s.Is(esclient.Restart) {
					restarts = append(restarts, s)
				}
			}
			if len(restarts) > maxConcurrentRestarts {
				observedErrors = append(observedErrors, fmt.Errorf("expected at most %d, got %d, restarts: %v", maxConcurrentRestarts, len(restarts), restarts))
			}
		},
		func(k *test.K8sClient, t *testing.T) { //nolint:thelper
			require.Empty(t, observedErrors)
		},
		func() bool {
			// do not run this check on versions that do not support node shutdown or non-HA clusters where we restart all nodes at once
			return version.MustParse(es.Spec.Version).LT(shutdown.MinVersion) || IsNonHASpec(es)
		},
	)
}
