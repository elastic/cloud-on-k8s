// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/runner/bucket"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/vault"
)

const (
	EKSDriverID                 = "eks"
	DefaultEKSRunConfigTemplate = `id: eks-dev
overrides:
  clusterName: %s-dev-cluster
  vaultInfo:
    address: %s
    token: %s
`
	EKSVaultPath            = "ci-aws-k8s-operator"
	eksDeleteRetryDelay     = 2 * time.Minute
	clusterCreationTemplate = `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: {{.ClusterName}}
  region: {{.Region}}
  version: "{{.KubernetesVersion}}"
  tags:
    {{- range $key, $value := .Tags }}
    {{ $key }}: {{ $value }}
    {{- end }}
nodeGroups:
  - name: ng-1
    amiFamily: AmazonLinux2023
    instanceType: {{.MachineType}}
    desiredCapacity: {{.NodeCount}}
    ami: {{.NodeAMI}}
    iam:
      instanceProfileARN: {{.InstanceProfileARN}}
      instanceRoleARN: {{.InstanceRoleARN}}
iam:
  withOIDC: false
  serviceRoleARN: {{.ServiceRoleARN}}
`
	awsAccessKeyID     = "aws_access_key_id"
	awsSecretAccessKey = "aws_secret_access_key" //nolint:gosec
	awsAuthTemplate    = `[default]
%s = %s
%s = %s`
)

func init() {
	drivers[EKSDriverID] = &EKSDriverFactory{}
}

type EKSDriverFactory struct{}

func (e EKSDriverFactory) Create(plan Plan) (Driver, error) {
	c, err := vault.NewClient()
	if err != nil {
		return nil, err
	}

	return &EKSDriver{
		plan: plan,
		ctx: map[string]any{
			"ClusterName":       plan.ClusterName,
			"Region":            plan.Eks.Region,
			"KubernetesVersion": plan.KubernetesVersion,
			"NodeCount":         plan.Eks.NodeCount,
			"MachineType":       plan.MachineType,
			"NodeAMI":           plan.Eks.NodeAMI,
			"WorkDir":           plan.Eks.WorkDir,
			"Tags":              elasticTags,
		},
		vaultClient: c,
	}, nil
}

var _ DriverFactory = (*EKSDriverFactory)(nil)

type EKSDriver struct {
	plan        Plan
	cleanUp     []func()
	ctx         map[string]any
	vaultClient vault.Client
}

func (e *EKSDriver) newCmd(cmd string) *exec.Command {
	return exec.NewCommand(cmd).
		AsTemplate(e.ctx)
}

func (e *EKSDriver) Execute() error {
	defer e.runCleanup()
	if err := e.auth(); err != nil {
		return err
	}
	exists, err := e.clusterExists()
	if err != nil {
		return fmt.Errorf("while checking cluster exists %w", err)
	}
	switch e.plan.Operation {
	case DeleteAction:
		// Track bucket deletion errors separately: cluster deletion should proceed even if bucket
		// deletion fails, but the error must still be returned so the exit code is non-zero.
		bucketErr := deleteBucketIfConfigured(e.plan, e.newBucketManager)
		if bucketErr != nil {
			log.Printf("warning: bucket deletion failed, will continue with cluster deletion: %v", bucketErr)
		}
		if exists {
			if err = e.delete(); err != nil {
				return errors.Join(err, bucketErr)
			}
		} else {
			log.Printf("Not deleting cluster as it does not exist")
		}
		if bucketErr != nil {
			return bucketErr
		}
	case CreateAction:
		//nolint:nestif
		if !exists {
			log.Printf("Creating cluster ...")
			if err := e.ensureWorkDir(); err != nil {
				return fmt.Errorf("while ensuring work dir %w", err)
			}
			var createCfg bytes.Buffer
			if err := template.Must(template.New("").Parse(clusterCreationTemplate)).Execute(&createCfg, e.ctx); err != nil {
				return fmt.Errorf("while formatting cluster create cfg %w", err)
			}
			createCfgFile := filepath.Join(e.ctx["WorkDir"].(string), "cluster.yaml") //nolint:forcetypeassert
			e.ctx["CreateCfgFile"] = createCfgFile
			if err := os.WriteFile(createCfgFile, createCfg.Bytes(), 0o600); err != nil {
				return fmt.Errorf("while writing create cfg %w", err)
			}
			if err := e.newCmd(`eksctl create cluster -v 1 -f {{.CreateCfgFile}}`).Run(); err != nil {
				return err
			}
		} else {
			log.Printf("Not creating cluster as it already exists")
		}

		// Always get credentials so kubectl targets the right cluster.
		if err := e.GetCredentials(); err != nil {
			return err
		}

		if err := setupDisks(e.plan); err != nil {
			return err
		}
		if err := createStorageClass(); err != nil {
			return err
		}
		if err := createBucketIfConfigured(e.plan, e.newBucketManager); err != nil {
			return err
		}
	}
	return nil
}

func (e *EKSDriver) runCleanup() {
	for _, f := range e.cleanUp {
		f()
	}
}

func (e *EKSDriver) ensureWorkDir() error {
	if e.ctx["WorkDir"] != "" {
		return nil
	}
	clusterName, ok := e.ctx["ClusterName"].(string)
	if !ok {
		return fmt.Errorf("expected ClusterName to be of type string, got type %T", e.ctx["ClusterName"])
	}
	dir, err := os.MkdirTemp("", clusterName)
	if err != nil {
		return err
	}

	e.ctx["WorkDir"] = dir
	e.cleanUp = append(e.cleanUp, func() {
		_ = os.RemoveAll(dir)
	})
	return nil
}

func (e *EKSDriver) GetCredentials() error {
	if err := e.auth(); err != nil {
		return err
	}
	log.Printf("Writing kubeconfig")
	// call `aws eks update-kubeconfig` instead of `eksctl utils write-kubeconfig` to write a valid kubeconfig for k8s >= 1.24 (https://github.com/aws/aws-cli/issues/6920)
	return e.newCmd("aws eks update-kubeconfig --name {{.ClusterName}} --region {{.Region}}").Run()
}

func (e *EKSDriver) clusterExists() (bool, error) {
	log.Printf("Checking if cluster exists ...")
	notFound, err := e.newCmd("eksctl get cluster --name {{.ClusterName}} --region {{.Region}}").WithoutStreaming().OutputContainsAny("No cluster found")
	if notFound {
		return false, nil
	}
	return err == nil, err
}

func (e *EKSDriver) auth() error {
	if err := e.fetchSecrets(); err != nil {
		return fmt.Errorf("while fetching secrets %w", err)
	}
	// while we could configure eksctl to take credentials from environment variables
	// we need to create a shared AWS credentials file for any subsequent kubectl commands to succeed
	if err := e.writeAWSCredentials(); err != nil {
		return fmt.Errorf("while writing .aws/credentials %w", err)
	}
	return nil
}

// fetchSecrets gets secret configuration data from vault and populates driver's context map with it.
func (e *EKSDriver) fetchSecrets() error {
	c, err := vault.NewClient()
	if err != nil {
		return err
	}
	for vaultKey, ctxKey := range map[string]string{
		"instance-profile": "InstanceProfileARN",
		"instance-role":    "InstanceRoleARN",
		"service-role":     "ServiceRoleARN",
		"access-key":       awsAccessKeyID,
		"secret-key":       awsSecretAccessKey,
	} {
		val, err := vault.Get(c, EKSVaultPath, vaultKey)
		if err != nil {
			return err
		}
		e.ctx[ctxKey] = val
	}
	return nil
}

func (e *EKSDriver) writeAWSCredentials() error {
	dir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ".aws")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = os.Mkdir(path, 0o600); err != nil {
			return err
		}
	}
	file := filepath.Join(path, "credentials")
	if _, err := os.Stat(file); err == nil {
		// don't overwrite existing credentials
		return nil
	}
	log.Printf("Writing aws credentials")
	fileContents := fmt.Sprintf(awsAuthTemplate, awsAccessKeyID, e.ctx[awsAccessKeyID], awsSecretAccessKey, e.ctx[awsSecretAccessKey])
	return os.WriteFile(file, []byte(fileContents), 0o600)
}

func (e *EKSDriver) newBucketManager() (bucket.Manager, error) {
	// Use VaultManager for pre-provisioned buckets
	if e.plan.Bucket.FromVault {
		// Determine if this is EKS ARM based on plan ID
		providerID := EKSDriverID
		if strings.Contains(e.plan.Id, "arm") {
			providerID = "eks-arm"
		}
		return newVaultBucketManager(providerID, e.vaultClient)
	}

	// Use S3Manager for dynamic bucket creation
	cfg, err := newBucketConfig(e.plan, e.ctx, e.plan.Eks.Region)
	if err != nil {
		return nil, err
	}
	if e.plan.Bucket.S3 == nil {
		return nil, fmt.Errorf("bucket.s3 configuration is required for EKS")
	}
	s3 := bucket.S3Config{
		IAMUserPath:      e.plan.Bucket.S3.IamUserPath,
		ManagedPolicyARN: e.plan.Bucket.S3.ManagedPolicyARN,
	}
	if err := bucket.ValidateShellArg(s3.IAMUserPath, "IAM user path"); err != nil {
		return nil, err
	}
	if err := bucket.ValidateShellArg(s3.ManagedPolicyARN, "managed policy ARN"); err != nil {
		return nil, err
	}
	return bucket.NewS3Manager(cfg, s3), nil
}

func (e *EKSDriver) Cleanup(prefix string, olderThan time.Duration) error {
	if err := e.auth(); err != nil {
		return err
	}

	sinceDate := time.Now().Add(-olderThan)
	e.ctx["Date"] = sinceDate.Format(time.RFC3339)
	e.ctx["E2EClusterNamePrefix"] = prefix

	allClustersCmd := `eksctl get cluster -r "{{.Region}}" -o json | jq -r 'map(select(.Name|test("{{.E2EClusterNamePrefix}}")))| .[].Name'`
	allClusters, err := exec.NewCommand(allClustersCmd).AsTemplate(e.ctx).OutputList()
	if err != nil {
		return err
	}

	describeClusterCmd := `aws eks describe-cluster --name "{{.ClusterName}}" --region "{{.Region}}" | jq -r --arg d "{{.Date}}" 'select(.cluster.createdAt | . <= $d) | .cluster.name'`

	for _, cluster := range allClusters {
		e.plan.ClusterName = cluster
		e.ctx["ClusterName"] = cluster
		clustersToDelete, err := exec.NewCommand(describeClusterCmd).AsTemplate(e.ctx).WithoutStreaming().Output()
		if err != nil {
			return fmt.Errorf("while describing cluster %s: %w", cluster, err)
		}
		if clustersToDelete != "" {
			if err := deleteBucketIfConfigured(e.plan, e.newBucketManager); err != nil {
				log.Printf("warning: bucket deletion failed for cluster %s, will continue: %v", cluster, err)
			}
			if err = e.delete(); err != nil {
				log.Printf("while deleting cluster %s: %v", cluster, err.Error())
				continue
			}
		}
	}

	return nil
}

func (e *EKSDriver) delete() error {
	// Retry because CloudFormation stack deletion can fail on resources that are released asynchronously,
	// e.g. ENIs still referencing the nodegroup security group while instances or load balancers terminate.
	// Re-running eksctl resumes the deletion of stacks left in DELETE_FAILED, even once the EKS cluster itself
	// is gone, as long as its cluster stack still exists.
	const maxDeleteAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxDeleteAttempts; attempt++ {
		if attempt > 1 {
			log.Printf("Retrying cluster deletion in %s (attempt %d/%d)...", eksDeleteRetryDelay, attempt, maxDeleteAttempts)
			time.Sleep(eksDeleteRetryDelay)
			exists, err := e.clusterOrStackExists()
			if err != nil {
				lastErr = err
				log.Printf("Error checking for cluster existence: %v", err)
				continue
			}
			if !exists {
				return nil
			}
		}
		log.Printf("Deleting cluster %s", e.ctx["ClusterName"])
		// --wait to surface failures to delete all resources in the Cloud formation
		// --disable-nodegroup-eviction to avoid pod disruption budgets causing nodegroup draining to fail.
		err := e.newCmd("eksctl delete cluster -v 1 --name {{.ClusterName}} --region {{.Region}} --wait --disable-nodegroup-eviction").Run()
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("Error deleting cluster: %v", err)
		e.logStackDeleteFailures()
	}
	return fmt.Errorf("failed to delete cluster after %d attempts: %w", maxDeleteAttempts, lastErr)
}

// clusterOrStackExists returns true if either the EKS cluster or its eksctl CloudFormation cluster stack
// still exists. A stack in DELETE_FAILED counts as existing.
func (e *EKSDriver) clusterOrStackExists() (bool, error) {
	exists, err := e.clusterExists()
	if err != nil || exists {
		return exists, err
	}
	cmd := `aws cloudformation list-stacks --region {{.Region}} ` +
		`--query "StackSummaries[?StackName=='eksctl-{{.ClusterName}}-cluster' && StackStatus!='DELETE_COMPLETE'].StackName" --output text`
	out, err := e.newCmd(cmd).WithoutStreaming().StdoutOnly().Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// logStackDeleteFailures prints the reasons reported by CloudFormation for resources it failed to delete,
// as eksctl only reports that the waiter transitioned to failure.
func (e *EKSDriver) logStackDeleteFailures() {
	for _, stack := range []string{"nodegroup-ng-1", "cluster"} {
		cmd := fmt.Sprintf(`aws cloudformation describe-stack-events --region {{.Region}} --stack-name eksctl-{{.ClusterName}}-%s `+
			`--query "StackEvents[?ResourceStatus=='DELETE_FAILED'].[Timestamp,LogicalResourceId,ResourceStatusReason]" --output text`, stack)
		if err := e.newCmd(cmd).Run(); err != nil {
			log.Printf("warning: could not describe stack events for %s: %v", stack, err)
		}
	}
}

var _ Driver = (*EKSDriver)(nil)
