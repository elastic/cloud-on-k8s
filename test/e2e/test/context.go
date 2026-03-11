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

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	logutil "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/stringsutil"
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
	Operator              NamespaceOperator `json:"operator"`
	E2EImage              string            `json:"e2e_image"`
	E2ENamespace          string            `json:"e2e_namespace"`
	E2EServiceAccount     string            `json:"e2e_service_account"`
	ElasticStackVersion   string            `json:"elastic_stack_version"`
	LogVerbosity          int               `json:"log_verbosity"`
	OperatorImage         string            `json:"operator_image"`
	OperatorImageRepo     string            `json:"operator_image_repo"`
	OperatorImageTag      string            `json:"operator_image_tag"`
	TestLicense           string            `json:"test_license"`
	TestLicensePKeyPath   string            `json:"test_license_pkey_path"`
	TestRegex             string            `json:"test_regex"`
	TestRun               string            `json:"test_run"`
	MonitoringSecrets     string            `json:"monitoring_secrets"`
	TestTimeout           time.Duration     `json:"test_timeout"`
	AutoPortForwarding    bool              `json:"auto_port_forwarding"`
	DeployChaosJob        bool              `json:"deploy_chaos_job"`
	Local                 bool              `json:"local"`
	IgnoreWebhookFailures bool              `json:"ignore_webhook_failures"`
	OcpCluster            bool              `json:"ocp_cluster"`
	AksCluster            bool              `json:"aks_cluster"`
	Pipeline              string            `json:"pipeline"`
	BuildNumber           string            `json:"build_number"`
	Provider              string            `json:"provider"`
	ClusterName           string            `json:"clusterName"`
	KubernetesVersion     version.Version   `json:"kubernetes_version"`
	GCPCredentialsPath    string            `json:"gcp_credentials_path"`
	AutopilotCluster      bool              `json:"autopilot_cluster"`
	TestEnvTags           []string          `json:"test_tags"`
	E2ETags               string            `json:"e2e_tags"`
	LogToFile             bool              `json:"log_to_file"`
	ArtefactsDir          string            `json:"artefacts_dir"`
	// DatePrefix is the date prefix for bucket paths (YYYYMMDD format).
	// Set once at context initialization to ensure consistency across a test run.
	DatePrefix            string            `json:"date_prefix"`
	// Stateless holds configuration for stateless Elasticsearch tests.
	// If nil, stateless mode is disabled.
	Stateless *StatelessConfig `json:"stateless,omitempty"`
}

// StatelessConfig holds configuration for stateless Elasticsearch tests.
type StatelessConfig struct {
	// Provider is the storage provider type: gcs, s3, or azure.
	Provider string `json:"provider"`
	// Bucket is the bucket name (GCS/S3) or container name (Azure).
	Bucket string `json:"bucket"`
	// StorageAccount is the Azure storage account name (only set for Azure provider).
	StorageAccount string `json:"storage_account,omitempty"`
	// SecretName is the name of the K8s Secret containing bucket credentials.
	SecretName string `json:"secret_name"`
	// SecretNamespace is the namespace where the bucket credentials Secret is located.
	// This Secret will be copied to managed namespaces when E2E tests run.
	SecretNamespace string `json:"secret_namespace"`
}

// IsEnabled returns true if stateless configuration is set (non-nil).
func (s *StatelessConfig) IsEnabled() bool {
	return s != nil
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

// IsStateless returns true if tests should run in stateless Elasticsearch mode.
func (c Context) IsStateless() bool {
	return c.Stateless.IsEnabled()
}

// TestBasePath returns the base path within the bucket for a test's data.
// Format: {DatePrefix}/{TestRun}/{testName}
// The date prefix (YYYYMMDD) makes cleanup easier - old directories can be identified
// and deleted by date, either manually or via scripts.
// This can be used by the Elasticsearch builder to construct the object store config,
// ensuring each stateless Elasticsearch instance has an isolated storage path.
func (c Context) TestBasePath(testName string) string {
	if c.Stateless == nil || c.Stateless.Bucket == "" {
		return ""
	}
	// DatePrefix is set once at context initialization to ensure all tests in a run
	// use the same date, even if the run spans midnight.
	datePrefix := c.DatePrefix
	if datePrefix == "" {
		// Fallback for contexts not initialized via the standard path (e.g., unit tests).
		datePrefix = time.Now().UTC().Format("20060102")
	}
	return fmt.Sprintf("%s/%s/%s", datePrefix, c.TestRun, testName)
}

// StatelessSecretRef returns the name and namespace of the bucket credentials Secret.
// This can be used by the Elasticsearch builder to automatically add the Secret
// to secureSettings for object store authentication.
// Returns empty strings if stateless mode is not enabled or not configured.
func (c Context) StatelessSecretRef() (name, namespace string) {
	if c.Stateless == nil {
		return "", ""
	}
	return c.Stateless.SecretName, c.Stateless.SecretNamespace
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
