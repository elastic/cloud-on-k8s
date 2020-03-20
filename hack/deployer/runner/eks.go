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
	awsAccessKey       = "AWS_ACCESS_KEY"        // nolint
	awsSecretAccessKey = "AWS_SECRET_ACCESS_KEY" // nolint
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
		AsTemplate(e.ctx).
		WithVariable(awsAccessKey, e.ctx[awsAccessKey].(string)).
		WithVariable(awsSecretAccessKey, e.ctx[awsSecretAccessKey].(string))
}

func (e *EKSDriver) Execute() error {
	defer e.runCleanup()
	if err := e.fetchSecrets(); err != nil {
		return fmt.Errorf("while fetching secrets %w", err)
	}

	exists, err := e.clusterExists()
	if err != nil {
		return fmt.Errorf("while checking cluster exists %w", err)
	}
	switch e.plan.Operation {
	case DeleteAction:
		if exists {
			log.Printf("Deleting cluster ...")
			return e.newCmd("eksctl delete cluster --name {{.ClusterName}} --region {{.Region}}").Run()
		}
		log.Printf("not deleting cluster as it does not exist")
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
			if err := ioutil.WriteFile(createCfgFile, createCfg.Bytes(), 0644); err != nil {
				return fmt.Errorf("while writing create cfg %w", err)
			}
			return e.newCmd(`eksctl create cluster -f {{.CreateCfgFile}}`).Run()
		}
		log.Printf("not creating cluster as it already exists")
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
	log.Printf("NOOP as eksctl populates ./kube/config by default")
	return nil
}

func (e *EKSDriver) clusterExists() (bool, error) {
	log.Printf("Checking if cluster exists ...")
	notFound, err := e.newCmd("eksctl get cluster --name {{.ClusterName}} --region {{.Region}}").WithoutStreaming().OutputContainsAny("No cluster found")
	if notFound {
		return false, nil
	}
	return err == nil, err
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
		"access-key":       awsAccessKey,
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

var _ Driver = &EKSDriver{}
