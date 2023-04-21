// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"log"
	"strings"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/runner/azure"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/vault"
)

func init() {
	drivers[AKSDriverID] = &AKSDriverFactory{}
}

const (
	AKSDriverID                    = "aks"
	AKSResourceGroupVaultFieldName = "resource-group"
	DefaultAKSRunConfigTemplate    = `id: aks-dev
overrides:
  clusterName: %s-dev-cluster
  aks:
    resourceGroup: %s
`
)

type AKSDriverFactory struct {
}

type AKSDriver struct {
	plan        Plan
	ctx         map[string]interface{}
	vaultClient vault.Client
}

func (gdf *AKSDriverFactory) Create(plan Plan) (Driver, error) {
	var c vault.Client
	// plan.ServiceAccount = true typically means a CI run vs a local run on a dev machine
	if plan.ServiceAccount {
		var err error
		c, err = vault.NewClient()
		if err != nil {
			return nil, err
		}

		if plan.Aks.ResourceGroup == "" {
			resourceGroup, err := vault.Get(c, azure.AKSVaultPath, AKSResourceGroupVaultFieldName)
			if err != nil {
				return nil, err
			}
			plan.Aks.ResourceGroup = resourceGroup
		}
	}

	return &AKSDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"ResourceGroup":     plan.Aks.ResourceGroup,
			"ClusterName":       plan.ClusterName,
			"NodeCount":         plan.Aks.NodeCount,
			"MachineType":       plan.MachineType,
			"KubernetesVersion": plan.KubernetesVersion,
			"Location":          plan.Aks.Location,
			"Zones":             plan.Aks.Zones,
		},
		vaultClient: c,
	}, nil
}

func (d *AKSDriver) Execute() error {
	if err := d.auth(); err != nil {
		return err
	}

	exists, err := d.clusterExists()
	if err != nil {
		return err
	}

	switch d.plan.Operation {
	case DeleteAction:
		if exists {
			if err := d.delete(); err != nil {
				return err
			}
		} else {
			log.Printf("not deleting as cluster doesn't exist")
		}
	case CreateAction:
		if exists {
			log.Printf("not creating as cluster exists")
		} else if err := d.create(); err != nil {
			return err
		}

		if err := d.GetCredentials(); err != nil {
			return err
		}

		if err := setupDisks(d.plan); err != nil {
			return err
		}
		if err := createStorageClass(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return nil
}

func (d *AKSDriver) auth() error {
	if d.plan.ServiceAccount {
		log.Print("Authenticating as service account...")
		credentials, err := azure.NewCredentials(d.vaultClient)
		if err != nil {
			return err
		}
		return azure.Login(credentials)
	}

	log.Print("Authenticating as user...")
	return exec.NewCommand("az login").Run()
}

func (d *AKSDriver) clusterExists() (bool, error) {
	log.Print("Checking if cluster exists...")

	cmd := azure.Cmd("aks", "show", "--name", d.plan.ClusterName, "--resource-group", d.plan.Aks.ResourceGroup)
	contains, err := cmd.WithoutStreaming().OutputContainsAny("not be found", "was not found")
	if contains {
		return false, nil
	}

	return err == nil, err
}

func (d *AKSDriver) create() error {
	log.Print("Creating cluster...")

	servicePrincipal := ""
	if d.plan.ServiceAccount {
		// our service principal doesn't have permissions to create a service principal for aks cluster
		// instead, we reuse the current service principal as the one for aks cluster
		secrets, err := vault.GetMany(d.vaultClient, azure.AKSVaultPath, "appId", "password")
		if err != nil {
			return err
		}
		servicePrincipal = fmt.Sprintf(" --service-principal %s --client-secret %s", secrets[0], secrets[1])
	}

	// https://learn.microsoft.com/en-us/cli/azure/aks?view=azure-cli-latest#az-aks-create
	return azure.Cmd("aks",
		"create", "--resource-group", d.plan.Aks.ResourceGroup,
		"--name", d.plan.ClusterName, "--location", d.plan.Aks.Location,
		"--node-count", fmt.Sprintf("%d", d.plan.Aks.NodeCount), "--node-vm-size", d.plan.MachineType,
		"--kubernetes-version", d.plan.KubernetesVersion,
		"--node-osdisk-size", "120", "--enable-addons", "http_application_routing", "--output", "none", "--generate-ssh-keys",
		"--zones", d.plan.Aks.Zones, servicePrincipal,
		"--tags", strings.Join(toList(elasticTags), " "),
	).Run()
}

func (d *AKSDriver) GetCredentials() error {
	if err := d.auth(); err != nil {
		return err
	}
	log.Print("Getting credentials...")
	return azure.Cmd("aks",
		"get-credentials", "--overwrite-existing",
		"--resource-group", d.plan.Aks.ResourceGroup,
		"--name", d.plan.ClusterName).
		Run()
}

func (d *AKSDriver) delete() error {
	log.Print("Deleting cluster...")
	return azure.Cmd("aks",
		"delete", "--yes",
		"--name", d.plan.ClusterName,
		"--resource-group", d.plan.Aks.ResourceGroup).
		Run()
}
