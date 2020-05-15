// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

const (
	EKSDriverID                 = "eks"
	EKSConfigFileName           = "deployer-config-eks.yml"
	DefaultEKSRunConfigTemplate = `id: eks-dev
overrides:
  clusterName: %s-dev-cluster
  vaultInfo:
    address: %s
    token: %s
`
	EKSVaultPath            = "secret/devops-ci/cloud-on-k8s/ci-aws-k8s-operator"
	clusterCreationTemplate = `apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: {{.ClusterName}}
  region: {{.Region}}
  version: "{{.KubernetesVersion}}"
nodeGroups:
  - name: ng-1
    instanceType: {{.MachineType}}
    desiredCapacity: {{.NodeCount}}
    ami: static
    iam:
      instanceProfileARN: {{.InstanceProfileARN}}
      instanceRoleARN: {{.InstanceRoleARN}}
iam:
  withOIDC: false
  serviceRoleARN: {{.ServiceRoleARN}}
`
	awsAccessKeyID      = "aws_access_key_id"
	awsSecretAccessKey  = "aws_secret_access_key" // nolint:gosec
	credentialsTemplate = `[default]
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
			"Region":            plan.EKS.Region,
			"KubernetesVersion": plan.KubernetesVersion,
			"NodeCount":         plan.EKS.NodeCount,
			"MachineType":       plan.MachineType,
			"NodeAMI":           plan.EKS.NodeAMI,
			"WorkDir":           plan.EKS.WorkDir,
		},
	}, nil
}

var _ DriverFactory = &EKSDriverFactory{}

type EKSDriver struct {
	plan    Plan
	cleanUp []func()
	ctx     map[string]interface{}
}

func (e *EKSDriver) newCmd(cmd string) *Command {
	return NewCommand(cmd).
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
			log.Printf("Deleting cluster ...")
			return e.newCmd("eksctl delete cluster -v 0 --name {{.ClusterName}} --region {{.Region}}").Run()
		}
		log.Printf("Not deleting cluster as it does not exist")
	case CreateAction:
		if !exists {
			log.Printf("Creating cluster ...")
			if err := e.ensureWorkDir(); err != nil {
				return fmt.Errorf("while ensuring work dir %w", err)
			}
			var createCfg bytes.Buffer
			if err := template.Must(template.New("").Parse(clusterCreationTemplate)).Execute(&createCfg, e.ctx); err != nil {
				return fmt.Errorf("while formatting cluster create cfg %w", err)
			}
			createCfgFile := filepath.Join(e.ctx["WorkDir"].(string), "cluster.yaml")
			e.ctx["CreateCfgFile"] = createCfgFile
			if err := ioutil.WriteFile(createCfgFile, createCfg.Bytes(), 0600); err != nil {
				return fmt.Errorf("while writing create cfg %w", err)
			}
			if err := e.newCmd(`eksctl create cluster -v 0 -f {{.CreateCfgFile}}`).Run(); err != nil {
				return err
			}
		} else {
			log.Printf("Not creating cluster as it already exists")
		}
		if err := createStorageClass(NoProvisioner); err != nil {
			return err
		}
		return NewCommand(e.plan.EKS.DiskSetup).Run()
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
	dir, err := ioutil.TempDir("", e.ctx["ClusterName"].(string))
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
	return e.newCmd("eksctl utils write-kubeconfig --cluster {{.ClusterName}} --region {{.Region}}").Run()
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
	client, err := NewClient(*e.plan.VaultInfo)
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
		val, err := client.Get(EKSVaultPath, vaultKey)
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
	fileContents := fmt.Sprintf(credentialsTemplate, awsAccessKeyID, e.ctx[awsAccessKeyID], awsSecretAccessKey, e.ctx[awsSecretAccessKey])
	return ioutil.WriteFile(file, []byte(fileContents), 0600)
}

var _ Driver = &EKSDriver{}
