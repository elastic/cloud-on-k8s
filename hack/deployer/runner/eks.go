// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
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
    amiFamily: AmazonLinux2
    instanceType: {{.MachineType}}
    desiredCapacity: {{.NodeCount}}
    ami: {{.NodeAMI}}
    overrideBootstrapCommand: |
      #!/bin/bash
      source /var/lib/cloud/scripts/eksctl/bootstrap.helper.sh

      /etc/eks/bootstrap.sh {{.ClusterName}} --container-runtime containerd --kubelet-extra-args "--node-labels=${NODE_LABELS}"
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

type EKSDriverFactory struct {
}

func (e EKSDriverFactory) Create(plan Plan) (Driver, error) {
	return &EKSDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"ClusterName":       plan.ClusterName,
			"Region":            plan.Eks.Region,
			"KubernetesVersion": plan.KubernetesVersion,
			"NodeCount":         plan.Eks.NodeCount,
			"MachineType":       plan.MachineType,
			"NodeAMI":           plan.Eks.NodeAMI,
			"WorkDir":           plan.Eks.WorkDir,
			"Tags":              elasticTags,
		},
	}, nil
}

var _ DriverFactory = &EKSDriverFactory{}

type EKSDriver struct {
	plan    Plan
	cleanUp []func()
	ctx     map[string]interface{}
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
		if exists {
			if err = e.delete(); err != nil {
				return err
			}
		}
		log.Printf("Not deleting cluster as it does not exist")
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
			if err := os.WriteFile(createCfgFile, createCfg.Bytes(), 0600); err != nil {
				return fmt.Errorf("while writing create cfg %w", err)
			}
			if err := e.newCmd(`eksctl create cluster -v 1 -f {{.CreateCfgFile}}`).Run(); err != nil {
				return err
			}
			if err := e.GetCredentials(); err != nil {
				return err
			}
		} else {
			log.Printf("Not creating cluster as it already exists")
		}

		if err := setupDisks(e.plan); err != nil {
			return err
		}
		return createStorageClass()
	}
	return nil
}

func (e *EKSDriver) runCleanup() func() {
	return func() {
		for _, f := range e.cleanUp {
			f()
		}
	}
}

func (e *EKSDriver) ensureWorkDir() error {
	if e.ctx["WorkDir"] != "" {
		return nil
	}
	dir, err := os.MkdirTemp("", e.ctx["ClusterName"].(string))
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
		if err = os.Mkdir(path, 0600); err != nil {
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
	return os.WriteFile(file, []byte(fileContents), 0600)
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
		e.ctx["ClusterName"] = cluster
		clustersToDelete, err := exec.NewCommand(describeClusterCmd).AsTemplate(e.ctx).WithoutStreaming().Output()
		if err != nil {
			return fmt.Errorf("while describing cluster %s: %w", cluster, err)
		}
		if clustersToDelete != "" {
			if err = e.delete(); err != nil {
				log.Printf("while deleting cluster %s: %v", cluster, err.Error())
				continue
			}
		}
	}

	return nil
}

func (e *EKSDriver) delete() error {
	log.Printf("Deleting cluster %s", e.ctx["ClusterName"])
	// --wait to surface failures to delete all resources in the Cloud formation
	// --disable-nodegroup-eviction to avoid pod disruption budgets causing nodegroup draining to fail.
	return e.newCmd("eksctl delete cluster -v 1 --name {{.ClusterName}} --region {{.Region}} --wait --disable-nodegroup-eviction").Run()
}

var _ Driver = &EKSDriver{}
