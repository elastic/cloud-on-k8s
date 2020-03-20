// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	logutil "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var defaultElasticStackVersion = LatestVersion7x

var (
	testContextPath = flag.String("testContextPath", "", "Path to the test context file")
	ctxInit         sync.Once
	ctx             Context
	log             logr.Logger
)

func init() {
	logutil.InitLogger()
	log = logf.Log.WithName("e2e")
}

// Ctx returns the current test context.
func Ctx() Context {
	ctxInit.Do(initializeContext)
	return ctx
}

func initializeContext() {
	if *testContextPath == "" {
		log.Info("No test context specified. Using defaults.")
		ctx = defaultContext()
		return
	}

	f, err := os.Open(*testContextPath)
	if err != nil {
		panic(fmt.Errorf("failed to open test context file %s: %v", *testContextPath, err))
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&ctx); err != nil {
		panic(fmt.Errorf("failed to decode test context: %v", err))
	}

	logutil.ChangeVerbosity(ctx.LogVerbosity)
	log.Info("Test context initialized", "context", ctx)
}

func defaultContext() Context {
	return Context{
		AutoPortForwarding:    false,
		ElasticStackVersion:   defaultElasticStackVersion,
		IgnoreWebhookFailures: false,
		Operator: NamespaceOperator{
			ClusterResource: ClusterResource{
				Name:      "elastic-operator",
				Namespace: "elastic-system",
			},
			ManagedNamespaces: []string{"mercury", "venus"},
		},
		TestRun:    "e2e-default",
		OcpCluster: false,
	}
}

// Context encapsulates data about a specific test run
type Context struct {
	Operator              NamespaceOperator `json:"operator"`
	E2EImage              string            `json:"e2e_image"`
	E2ENamespace          string            `json:"e2e_namespace"`
	E2EServiceAccount     string            `json:"e2e_service_account"`
	ElasticStackVersion   string            `json:"elastic_stack_version"`
	LogVerbosity          int               `json:"log_verbosity"`
	OperatorImage         string            `json:"operator_image"`
	TestLicense           string            `json:"test_license"`
	TestRegex             string            `json:"test_regex"`
	TestRun               string            `json:"test_run"`
	MonitoringSecrets     string            `json:"monitoring_secrets"`
	TestTimeout           time.Duration     `json:"test_timeout"`
	AutoPortForwarding    bool              `json:"auto_port_forwarding"`
	Local                 bool              `json:"local"`
	IgnoreWebhookFailures bool              `json:"ignore_webhook_failures"`
	OcpCluster            bool              `json:"ocp_cluster"`
	Pipeline              string            `json:"pipeline"`
	BuildNumber           string            `json:"build_number"`
	Provider              string            `json:"provider"`
	ClusterName           string            `json:"clusterName"`
	KubernetesVersion     string            `json:"kubernetes_version"`
}

// ManagedNamespace returns the nth managed namespace.
func (c Context) ManagedNamespace(n int) string {
	return c.Operator.ManagedNamespaces[n]
}

// ClusterResource is a generic cluster resource.
type ClusterResource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// NamespaceOperator is cluster resource with an associated namespace to manage.
type NamespaceOperator struct {
	ClusterResource
	ManagedNamespaces []string `json:"managed_namespaces"`
}
