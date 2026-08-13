// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	esuser "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	testlogstash "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/logstash"
)

// TestElasticsearchRoleOverride verifies that setting ElasticsearchRole on Agent and Logstash
// ES refs overrides the default ECK-managed role for the associated file-realm user.
func TestElasticsearchRoleOverride(t *testing.T) {
	name := "test-role-override"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	lsBuilder := testlogstash.NewBuilder("test-role-override-ls").
		WithNamespace(esBuilder.Elasticsearch.Namespace).
		WithNodeCount(1).
		WithElasticsearchRefs(logstashv1alpha1.ElasticsearchCluster{
			ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esBuilder.Ref()},
			ClusterName:           "default",
			ElasticsearchRole:     esuser.SuperUserBuiltinRole,
		})

	standaloneAgentBuilder := agent.NewBuilder("test-role-override-agent").
		WithNamespace(esBuilder.Elasticsearch.Namespace).
		WithDeployment().
		WithElasticsearchRefs(agent.ToOutputWithRole(esBuilder.Ref(), "default", esuser.SuperUserBuiltinRole))

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Standalone Agent with ElasticsearchRole:superuser should acquire superuser privileges",
				Test: func(t *testing.T) {
					t.Helper()

					username := association.ElasticsearchUserName(standaloneAgentBuilder.Agent.GetAssociations()[0], "agent-user")

					test.Eventually(func() error {
						return verifyUserWithSuperUserRole(t.Context(), k, esBuilder, username)
					})(t)
				},
			},
			{
				Name: "Logstash with ElasticsearchRole:superuser should acquire superuser privileges",
				Test: func(t *testing.T) {
					t.Helper()

					username := association.ElasticsearchUserName(lsBuilder.Logstash.GetAssociations()[0], "logstash-user")

					test.Eventually(func() error {
						return verifyUserWithSuperUserRole(t.Context(), k, esBuilder, username)
					})(t)
				},
			},
		}
	})

	test.Sequence(nil, steps, esBuilder, standaloneAgentBuilder, lsBuilder).RunSequential(t)
}

func verifyUserWithSuperUserRole(ctx context.Context, k *test.K8sClient, esBuilder elasticsearch.Builder, username string) error {
	esClient, err := elasticsearch.NewElasticsearchClient(esBuilder.Elasticsearch, k)
	if err != nil {
		return err
	}

	resp, err := elasticsearch.HasPrivilegesAs(ctx, esClient, username, `{"cluster":["monitor","manage"]}`)
	if err != nil {
		return fmt.Errorf("_has_privileges for %s: %w", username, err)
	}

	if !resp.Cluster["monitor"] {
		return fmt.Errorf("user %s: cluster:monitor=false, user not yet active", username)
	}

	if !resp.Cluster["manage"] {
		return fmt.Errorf("user %s: cluster:manage=false, expected superuser role override", username)
	}

	auth, err := elasticsearch.Authenticate(context.Background(), esClient, username)
	if err != nil {
		return fmt.Errorf("authenticate call for %s in ES %s: %w", username, esBuilder.Elasticsearch.Name, err)
	}
	if !auth.Enabled || !slices.Contains(auth.Roles, user.SuperUserBuiltinRole) {
		return fmt.Errorf("agent user %s should not have the role %s", username, user.SuperUserBuiltinRole)
	}
	return nil
}
