// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	logutil "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

const (
	// ArchARMTag is the test tag used to indicate a test run on an ARM-based cluster.
	ArchARMTag = "arch:arm"
)

var defaultElasticStackVersion = LatestReleasedVersion7x

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
		panic(fmt.Errorf("failed to open test context file %s: %w", *testContextPath, err))
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&ctx); err != nil {
		panic(fmt.Errorf("failed to decode test context: %w", err))
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

type ElasticStackImageDefinition struct {
	Kind    string `json:"kind"`
	Image   string `json:"image"`
	Version string `json:"version"`
}

type ElasticStackImages []ElasticStackImageDefinition

// Context encapsulates data about a specific test run
type Context struct {
	Operator              NamespaceOperator  `json:"operator"`
	E2EImage              string             `json:"e2e_image"`
	E2ENamespace          string             `json:"e2e_namespace"`
	E2EServiceAccount     string             `json:"e2e_service_account"`
	ElasticStackVersion   string             `json:"elastic_stack_version"`
	ElasticStackImages    ElasticStackImages `json:"elastic_stack_images"`
	LogVerbosity          int                `json:"log_verbosity"`
	OperatorImage         string             `json:"operator_image"`
	OperatorImageRepo     string             `json:"operator_image_repo"`
	OperatorImageTag      string             `json:"operator_image_tag"`
	TestLicense           string             `json:"test_license"`
	TestLicensePKeyPath   string             `json:"test_license_pkey_path"`
	TestRegex             string             `json:"test_regex"`
	TestRun               string             `json:"test_run"`
	MonitoringSecrets     string             `json:"monitoring_secrets"`
	TestTimeout           time.Duration      `json:"test_timeout"`
	AutoPortForwarding    bool               `json:"auto_port_forwarding"`
	DeployChaosJob        bool               `json:"deploy_chaos_job"`
	Local                 bool               `json:"local"`
	IgnoreWebhookFailures bool               `json:"ignore_webhook_failures"`
	OcpCluster            bool               `json:"ocp_cluster"`
	Ocp3Cluster           bool               `json:"ocp3_cluster"`
	Pipeline              string             `json:"pipeline"`
	BuildNumber           string             `json:"build_number"`
	Provider              string             `json:"provider"`
	ClusterName           string             `json:"clusterName"`
	KubernetesVersion     version.Version    `json:"kubernetes_version"`
	TestEnvTags           []string           `json:"test_tags"`
}

// ManagedNamespace returns the nth managed namespace.
func (c Context) ManagedNamespace(n int) string {
	return c.Operator.ManagedNamespaces[n]
}

// HasTag returns true if the test tags contain the specified value.
func (c Context) HasTag(tag string) bool {
	return stringsutil.StringInSlice(tag, c.TestEnvTags)
}

// KubernetesMajorMinor returns just the major and minor version numbers of the effective Kubernetes version.
func (c Context) KubernetesMajorMinor() string {
	return fmt.Sprintf("%d.%d", c.KubernetesVersion.Major, c.KubernetesVersion.Minor)
}

// ImageDefinitionFor returns a specific override for the given kind of resource. Defaults to an empty image
// and the global Elastic Stack version under test if no override exists.
func (c Context) ImageDefinitionFor(kind string) ElasticStackImageDefinition {
	for _, def := range c.ElasticStackImages {
		if kind == def.Kind {
			return def
		}
	}
	return ElasticStackImageDefinition{Version: c.ElasticStackVersion}
}

// ClusterResource is a generic cluster resource.
type ClusterResource struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// NamespaceOperator is cluster resource with an associated namespace to manage.
type NamespaceOperator struct {
	ClusterResource
	Replicas          int      `json:"operator_replicas"`
	ManagedNamespaces []string `json:"managed_namespaces"`
}
