// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build kb e2e

package kb

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	kibana2 "github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/telemetry"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTelemetry(t *testing.T) {
	name := "test-telemetry"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	// Kibana picks up static telemetry data from telemetry.yml very close to its start and then rereads it on a rather
	// long interval (~hours). This means that the telemetry reporter is likely to be updating Kibana config Secret
	// too late. To overcome that, we wait until the Secret is populated as expected, then we delete the Pod so
	// it will be restarted. Kibana will then pick up the telemetry.yml file correctly and can validate that via API.
	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Kibana config Secret should contain telemetry.yml key",
				Test: test.Eventually(func() error {
					var secret corev1.Secret
					err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: kbBuilder.Kibana.Namespace,
						Name:      kibana2.SecretName(kbBuilder.Kibana),
					}, &secret)
					if err != nil {
						return err
					}

					if content, ok := secret.Data["telemetry.yml"]; !ok || len(content) == 0 {
						return fmt.Errorf(
							"telemetry.yml key not found or empty in %s/%s",
							secret.Namespace,
							secret.Name)
					}

					return nil
				}),
			},
			{
				Name: "Restart Kibana Pod for Kibana to pick up static telemetry data",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(
						client.InNamespace(kbBuilder.Kibana.Namespace),
						client.MatchingLabels{"kibana.k8s.elastic.co/name=": kbBuilder.Kibana.Name},
					)
					if err != nil {
						return err
					}

					require.Equal(t, 1, len(pods))
					if err := k.DeletePod(pods[0]); err != nil {
						return err
					}

					return nil
				}),
			},
			{
				Name: "Kibana should expose eck info in telemetry data",
				Test: test.Eventually(func() error {
					body, err := kibana.MakeTelemetryRequest(kbBuilder, k)
					if err != nil {
						return err
					}

					var stats clusterStats
					err = json.Unmarshal(body, &stats)
					if err != nil {
						return err
					}
					if len(stats) == 0 {
						return errors.New("cluster stats is empty")
					}
					eck := stats[0].StackStats.Kibana.Plugins.StaticTelemetry.ECK
					if !eck.IsDefined() {
						return fmt.Errorf("eck info not defined properly in telemetry data: %+v", eck)
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(nil, stepsFn, esBuilder, kbBuilder).RunSequential(t)

}

// clusterStats partially models the response from a request to /api/telemetry/v1/clusters/_stats
type clusterStats []struct {
	StackStats struct {
		Kibana struct {
			Plugins struct {
				StaticTelemetry struct {
					telemetry.ECKTelemetry
				} `json:"static_telemetry"`
			} `json:"plugins"`
		} `json:"kibana"`
	} `json:"stack_stats"`
}
