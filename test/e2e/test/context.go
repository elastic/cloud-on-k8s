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

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const defaultElasticStackVersion = "7.2.0"

var (
	testContextPath = flag.String("testContextPath", "", "Path to the test context file")
	ctxInit         sync.Once
	ctx             Context
	log             = logf.Log.WithName("e2e")
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
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

	log.Info("Test context initialized", "context", ctx)
}

func defaultContext() Context {
	return Context{
		AutoPortForwarding:  false,
		ElasticStackVersion: defaultElasticStackVersion,
		GlobalOperator: ClusterResource{
			Name:      "elastic-global-operator",
			Namespace: "elastic-system",
		},
		NamespaceOperators: []NamespaceOperator{
			{
				ClusterResource: ClusterResource{
					Name:      "mercury-ns-operator",
					Namespace: "elastic-ns-operators",
				},
				ManagedNamespace: "mercury",
			},
			{
				ClusterResource: ClusterResource{
					Name:      "venus-ns-operator",
					Namespace: "elastic-ns-operators",
				},
				ManagedNamespace: "venus",
			},
		},
		TestRun: "e2e-default",
	}
}

// Context encapsulates data about a specific test run
type Context struct {
	GlobalOperator      ClusterResource     `json:"global_operator"`
	NamespaceOperators  []NamespaceOperator `json:"namespace_operators"`
	E2EImage            string              `json:"e2e_image"`
	E2ENamespace        string              `json:"e2e_namespace"`
	E2EServiceAccount   string              `json:"e2e_service_account"`
	ElasticStackVersion string              `json:"elastic_stack_version"`
	OperatorImage       string              `json:"operator_image"`
	TestLicence         string              `json:"test_licence"`
	TestRegex           string              `json:"test_regex"`
	TestRun             string              `json:"test_run"`
	AutoPortForwarding  bool                `json:"auto_port_forwarding"`
	Local               bool                `json:"local"`
}

// ManagedNamespace returns the nth managed namespace.
func (c Context) ManagedNamespace(n int) string {
	return c.NamespaceOperators[n].ManagedNamespace
}

// OperatorNamespaces returns the unique set of namespaces that have operators deployed.
func (c Context) OperatorNamespaces() []string {
	seen := make(map[string]struct{}, len(c.NamespaceOperators))
	for _, ns := range c.NamespaceOperators {
		seen[ns.Namespace] = struct{}{}
	}

	namespaces := make([]string, 0, len(seen))
	for ns := range seen {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

// ClusterResource is a generic cluster resource.
type ClusterResource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// NamespaceOperator is cluster resource with an associated namespace to manage.
type NamespaceOperator struct {
	ClusterResource
	ManagedNamespace string `json:"managed_namespace"`
}
